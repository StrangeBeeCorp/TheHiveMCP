package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/lint"
)

const (
	root         = "/repo"
	golangci     = "golangci/golangci-lint:v2.12.2"
	goTestImage  = "golang:1.26.8"
	goImage      = "golang:1.26.8-alpine"
	module       = "./..."
	appMD        = "/app/" + mdFile
	stepShfmt    = "shfmt"
	stepGofmt    = "gofmt"
	stepPrettie  = "prettier"
	stepGolangci = "golangci-lint"
	dockerCmd    = "docker"
	runCmd       = "run"
	stepShell    = "shellcheck"
	stepMarkdown = "markdownlint-cli2"
	rm           = "--rm"
	appDir       = "/app"
	mount        = root + ":" + appDir
)

var (
	repo = lint.Repo{Root: root, Images: lint.Images{Go: goImage, GoTest: goTestImage}, CheckmakeINI: []byte("[maxbodylength]\nmaxBodyLength = 20\n")}
	// every kind of file once, a fragment included
	everyKind = lint.Classify([]string{goFile, shFile, mdFile, ciFile, dockerf, makefile, mkFile})
)

func names(steps []lint.Step) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.Name
	}

	return out
}

func byName(t *testing.T, steps []lint.Step, name string) lint.Step {
	t.Helper()

	for _, s := range steps {
		if s.Name == name {
			return s
		}
	}

	require.Failf(t, "no step", "%q in %v", name, names(steps))

	return lint.Step{}
}

// A check reads every kind of file with its tool, and reports what the formatters would change.
func TestLintStepsCheckEveryKindOfFile(t *testing.T) {
	t.Parallel()

	steps := lint.Steps(everyKind, repo, false)

	assert.Equal(t, []string{
		"gofmt -l (needs formatting; run make fmt)", stepGolangci, "go test -short", stepShell,
		"shfmt (needs formatting; run make fmt)", stepMarkdown, "lychee", "yamllint",
		"hadolint (deployment/Dockerfile)", "checkmake", "checkmake (fragments)", "actionlint",
	}, names(steps))

	for _, s := range steps {
		assert.Equal(t, lint.Check, s.Fix, s.Name)
	}

	assert.Equal(t, []string{
		dockerCmd, runCmd, rm, "-v", mount, "-w", appDir,
		"-v", "thehivemcp-gomod:/go/pkg/mod", "-v", "thehivemcp-gobuild:/root/.cache/go-build", "-v", "thehivemcp-golangci-cache:/root/.cache/golangci-lint",
		"-e", "GOOS=linux", "-e", "GOARCH=amd64", // type-checked as CI builds it, whatever this machine is
		golangci, stepGolangci, "run", module,
	}, byName(t, steps, stepGolangci).Argv, "the whole module: cross-file findings")
	assert.Equal(t, []string{
		dockerCmd, runCmd, rm, "-v", mount, "-w", appDir,
		"-v", "thehivemcp-gomod:/go/pkg/mod", "-v", "thehivemcp-gobuild:/root/.cache/go-build", "-e", "CGO_ENABLED=0",
		goTestImage, "go", "test", "-short", module,
	}, byName(t, steps, "go test -short").Argv, "make test's image, which has git")

	gofmt := byName(t, steps, "gofmt -l (needs formatting; run make fmt)")
	assert.True(t, gofmt.Lists)
	assert.Equal(t, []string{"gofmt", "-l", "/app/internal/x/x.go"}, gofmt.Argv[len(gofmt.Argv)-3:])
	assert.Equal(t, []string{"--offline", mdFile}, tail(byName(t, steps, "lychee").Argv, 2), "local links only")
	assert.Contains(t, byName(t, steps, "lychee").Argv, "--init")
	assert.Equal(t, []string{"-no-color", ciFile}, tail(byName(t, steps, "actionlint").Argv, 2))
	assert.Equal(t, []string{stepMarkdown, appMD}, tail(byName(t, steps, stepMarkdown).Argv, 2))

	assert.Equal(t, []string{"koalaman/shellcheck:v0.11.0", shFile}, tail(byName(t, steps, stepShell).Argv, 2))
	assert.Equal(t, []string{"mvdan/shfmt:v3.13.1", "-i", "2", "-ci", "-l", shFile}, tail(byName(t, steps, "shfmt (needs formatting; run make fmt)").Argv, 6))

	yl := byName(t, steps, "yamllint").Argv
	assert.Equal(t, ciFile, yl[len(yl)-1])
	assert.Contains(t, yl[len(yl)-2], "line-length: disable")
}

// Dockerfiles go to hadolint on stdin, one each; the Makefile gets checkmake.ini, a fragment the
// same rules without minphony.
func TestDockerfilesAndMakefilesGetTheirConfiguration(t *testing.T) {
	t.Parallel()

	steps := lint.Steps(everyKind, repo, false)

	hadolint := byName(t, steps, "hadolint (deployment/Dockerfile)")
	assert.Equal(t, dockerf, hadolint.Stdin)
	assert.Equal(t, []string{dockerCmd, runCmd, rm, "-i", "hadolint/hadolint:v2.14.0", "hadolint", "-"}, hadolint.Argv)

	withConfig := repo
	withConfig.HadolintConfig = true
	assert.Equal(t, []string{
		dockerCmd, runCmd, rm, "-i", "-v", "/repo/.hadolint.yaml:/.hadolint.yaml:ro",
		"hadolint/hadolint:v2.14.0", "hadolint", "--config", "/.hadolint.yaml", "-",
	}, byName(t, lint.Steps(everyKind, withConfig, false), "hadolint (deployment/Dockerfile)").Argv)

	whole := byName(t, steps, "checkmake")
	assert.Contains(t, whole.Argv, "/repo/checkmake.ini:/checkmake.ini:ro")
	assert.Equal(t, []string{"--config=/checkmake.ini", "/app/Makefile"}, tail(whole.Argv, 2))
	assert.Nil(t, whole.Before)

	part := byName(t, steps, "checkmake (fragments)")
	assert.Contains(t, part.Argv, "/repo/"+lint.FragmentConfig+":/checkmake.ini:ro")
	assert.Equal(t, []string{"--config=/checkmake.ini", "/app/scripts/harness.mk"}, tail(part.Argv, 2))
	require.NotNil(t, part.Before)
	assert.Equal(t, lint.FragmentConfig, part.Before.Path)
	assert.Equal(t, "[maxbodylength]\nmaxBodyLength = 20\n\n[minphony]\ndisabled = true\n", string(part.Before.Content))

	noINI := repo
	noINI.CheckmakeINI = nil
	assert.Equal(t, []string{
		dockerCmd, runCmd, rm, "-v", mount, "-w", appDir, "--entrypoint", "/checkmake",
		"quay.io/checkmake/checkmake@sha256:6db49d6b62e2abed57c2fd334ca5f43067126bd9419a71b13967bd0eccf6cdcb", "/app/Makefile",
	}, byName(t, lint.Steps(everyKind, noINI, false), "checkmake").Argv, "without checkmake.ini, its defaults")
}

// With fix, the formatters and golangci-lint's safe fixes run first, and the checks that would
// only repeat them do not.
func TestLintFixFormatsThenFixesThenChecks(t *testing.T) {
	t.Parallel()

	steps := lint.Steps(everyKind, repo, true)

	assert.Equal(t, []string{
		stepGofmt, "golangci-lint fmt", stepShfmt, "markdownlint-cli2 --fix", stepPrettie,
		"golangci-lint run --fix", stepGolangci, "go test -short", stepShell, "lychee", "yamllint",
		"hadolint (deployment/Dockerfile)", "checkmake", "checkmake (fragments)", "actionlint",
	}, names(steps))
	assert.Equal(t, lint.Format, byName(t, steps, stepPrettie).Fix)
	assert.Equal(t, lint.LintFix, byName(t, steps, "golangci-lint run --fix").Fix)
	assert.Equal(t, []string{golangci, stepGolangci, "run", "--fix", module}, tail(byName(t, steps, "golangci-lint run --fix").Argv, 5))
	assert.Equal(t, lint.Check, byName(t, steps, stepGolangci).Fix)
}

func TestFormatStepsRewriteOrList(t *testing.T) {
	t.Parallel()

	check := lint.FormatSteps(everyKind, repo, false)
	assert.Equal(t, []string{"gofmt (would reformat)", "shfmt (would reformat)", "prettier (would reformat)"}, names(check))

	for _, s := range check {
		assert.True(t, s.Lists, s.Name)
	}

	assert.Equal(t, []string{"npx", "--yes", "prettier@3.9.5", "--list-different", appMD}, tail(byName(t, check, "prettier (would reformat)").Argv, 5),
		"one prettier version on every host")

	fix := lint.FormatSteps(everyKind, repo, true)
	assert.Equal(t, []string{"mvdan/shfmt:v3.13.1", "-i", "2", "-ci", "-l", "-w", shFile}, tail(byName(t, fix, stepShfmt).Argv, 7))
	assert.Equal(t, []string{goImage, "gofmt", "-l", "-w", "/app/internal/x/x.go"}, tail(byName(t, fix, stepGofmt).Argv, 5))
	assert.Equal(t, []string{golangci, stepGolangci, "fmt", module}, tail(byName(t, fix, "golangci-lint fmt").Argv, 4), "the whole module, as golangci-lint fmt sees it")
	assert.Equal(t, []string{"davidanson/markdownlint-cli2:v0.23.0", stepMarkdown, "--fix", appMD}, tail(byName(t, fix, "markdownlint-cli2 --fix").Argv, 4))
	assert.Equal(t, []string{"-e", "npm_config_update_notifier=false", "node:22-alpine", "npx", "--yes", "prettier@3.9.5", "--write", appMD},
		tail(byName(t, fix, stepPrettie).Argv, 8))

	assert.Empty(t, lint.FormatSteps(lint.Files{}, repo, true), "no file, no tool")
	assert.Empty(t, lint.Steps(lint.Files{}, repo, false))
}

func TestMakefileImagesReadsTheGoPins(t *testing.T) {
	t.Parallel()

	im, err := lint.MakefileImages([]byte("GO_IMAGE := golang:1.26.8-alpine\nGO_IMAGE_CGO := golang:1.26.8\nOTHER_IMAGE := x:1\n"))
	require.NoError(t, err)
	assert.Equal(t, lint.Images{Go: goImage, GoTest: goTestImage}, im)

	_, err = lint.MakefileImages([]byte("GO_IMAGE := golang:1.26.8-alpine\n"))
	require.Error(t, err)
}

func tail(s []string, n int) []string {
	return s[len(s)-n:]
}
