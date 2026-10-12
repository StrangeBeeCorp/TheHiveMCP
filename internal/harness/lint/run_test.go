package lint_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/lint"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

// fakeDocker answers each command line by the tool it names (its last word before the args is
// enough here: the step's name is its first word), and records what it was asked.
type fakeDocker struct {
	mu      sync.Mutex
	calls   [][]string
	stdins  map[string]string
	answers map[string]answer // by tool word
	have    map[string]bool   // images docker already has
}

type answer struct {
	out    string
	stderr string
	code   int
	err    error
	do     func()
}

func (f *fakeDocker) exec(_ context.Context, argv []string, stdin io.Reader) (proc.Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, argv)

	if argv[1] == "image" {
		if f.have[argv[len(argv)-1]] {
			return proc.Output{Stdout: []byte("sha256:x")}, nil
		}

		return proc.Output{Stderr: []byte("No such image"), Code: 1}, nil
	}

	if argv[1] == "pull" {
		return proc.Output{}, nil
	}

	for word, a := range f.answers {
		if strings.Contains(strings.Join(argv, " "), word) {
			if stdin != nil {
				data, _ := io.ReadAll(stdin)
				f.stdins[word] = string(data)
			}

			if a.do != nil {
				a.do()
			}

			return proc.Output{Stdout: []byte(a.out), Stderr: []byte(a.stderr), Code: a.code}, a.err
		}
	}

	return proc.Output{}, nil
}

func (f *fakeDocker) ran(prefix ...string) int {
	n := 0

	for _, c := range f.calls {
		if len(c) >= len(prefix) && strings.Join(c[:len(prefix)], " ") == strings.Join(prefix, " ") {
			n++
		}
	}

	return n
}

const xGo = "x.go"

func newFake() *fakeDocker {
	return &fakeDocker{stdins: map[string]string{}, answers: map[string]answer{}, have: map[string]bool{}}
}

// A check fails on a non-zero exit, and a listing tool on any output, with the container's /app/
// taken off its paths; a tool that cannot start is a failure, never a pass.
func TestRunReadsEachChecksResult(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.answers[stepShell] = answer{out: "In scripts/a.sh line 1: SC2086\n", code: 1}
	fake.answers["gofmt"] = answer{out: "/app/internal/x/x.go\n"}
	fake.answers["shfmt"] = answer{stderr: "a notice on stderr, which lists no file\n"}
	fake.answers["markdownlint"] = answer{err: errors.New("exec: docker: not found")}

	steps := lint.Steps(lint.Classify([]string{goFile, shFile, mdFile}), repo, false)
	rep := lint.Runner{Root: t.TempDir(), Exec: fake.exec}.Run(t.Context(), steps)

	assert.Equal(t, []lint.Failure{
		{Name: "gofmt -l (needs formatting; run make fmt)", Output: "internal/x/x.go"},
		{Name: stepShell, Output: "In scripts/a.sh line 1: SC2086"},
		{Name: stepMarkdown, Output: "markdownlint-cli2 could not run: exec: docker: not found"},
	}, rep.Failures, "golangci-lint, go test, shfmt and lychee passed")
}

// A listing tool that fails without listing a file (prettier exits 2 on its own error) is a
// failure with what it printed.
func TestRunReportsAListingToolsOwnError(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.answers["prettier"] = answer{stderr: "npm notice\n[error] Cannot find package\n", code: 2}
	fake.answers["gofmt"] = answer{out: "/app/b.go\n", stderr: "a.go:1:1: expected 'package'\n", code: 2}

	rep := lint.Runner{Root: t.TempDir(), Exec: fake.exec}.Run(t.Context(), lint.FormatSteps(lint.Classify([]string{"a.go", "b.go", mdFile}), repo, false))

	assert.Equal(t, []lint.Failure{
		{Name: "gofmt (would reformat)", Output: "b.go"},
		{Name: "prettier (would reformat)", Output: "npm notice\n[error] Cannot find package"},
	}, rep.Failures)
}

// A fix's exit and output are no finding; the files it changed are reported, by kind of fix.
func TestRunReportsTheFilesEachFixChanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	gofile := filepath.Join(dir, xGo)
	require.NoError(t, os.WriteFile(gofile, []byte("package x\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "y.go"), []byte("package x\n"), 0o600))

	fake := newFake()
	fake.answers["gofmt -l -w"] = answer{code: 2, out: "noise", do: func() { _ = os.WriteFile(gofile, []byte("package x\n\n"), 0o600) }}
	fake.answers["run --fix"] = answer{do: func() { _ = os.WriteFile(filepath.Join(dir, "y.go"), []byte("package y\n"), 0o600) }}

	steps := lint.Steps(lint.Classify([]string{xGo}), repo, true)
	rep := lint.Runner{Root: dir, Exec: fake.exec, Watch: []string{xGo, "y.go", "gone.go"}}.Run(t.Context(), steps)

	assert.Empty(t, rep.Failures)
	assert.Equal(t, []string{xGo}, rep.Fixed[lint.Format])
	assert.Equal(t, []string{"y.go"}, rep.Fixed[lint.LintFix], "a safe fix outside the scope is reported too")
}

// A fix that docker could not run (its own exit codes, 125 and up) or that could not start is a
// failure; one that ran and left something unfixed is not, since the checks after it report that.
func TestRunFailsAFixThatCouldNotRun(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.answers["markdownlint-cli2 --fix"] = answer{out: "MD013 cannot be fixed\n", code: 1}
	fake.answers["prettier@"] = answer{stderr: "docker: Error response from daemon: pull access denied\n", code: 125}
	fake.answers["shfmt"] = answer{err: errors.New("exec: docker: not found")}

	rep := lint.Runner{Root: t.TempDir(), Exec: fake.exec}.Run(t.Context(), lint.FormatSteps(lint.Classify([]string{shFile, mdFile}), repo, true))

	assert.Equal(t, []lint.Failure{
		{Name: stepShfmt, Output: "shfmt could not run: exec: docker: not found"},
		{Name: stepPrettie, Output: "docker: Error response from daemon: pull access denied"},
	}, rep.Failures)
}

// A fix killed before it finished (a signal, a cancelled gate: a negative exit code) is a failure,
// not a clean run that formatted nothing.
func TestRunFailsAFixThatWasKilled(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.answers["markdownlint-cli2 --fix"] = answer{stderr: "signal: killed\n", code: -1}

	rep := lint.Runner{Root: t.TempDir(), Exec: fake.exec}.Run(t.Context(), lint.FormatSteps(lint.Classify([]string{mdFile}), repo, true))

	require.Len(t, rep.Failures, 1)
	assert.Contains(t, rep.Failures[0].Name, "markdownlint")
	assert.Equal(t, "signal: killed", rep.Failures[0].Output)
}

// An image docker has is not pulled again; one it lacks is pulled quietly, once, before the
// steps run, so its progress never reads as a finding.
func TestRunPullsOnlyTheMissingImages(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.have[golangci] = true

	lint.Runner{Root: t.TempDir(), Exec: fake.exec}.Run(t.Context(), lint.Steps(lint.Classify([]string{goFile}), repo, false))

	assert.Equal(t, 0, fake.ran(dockerCmd, "pull", "-q", golangci))
	assert.Equal(t, 1, fake.ran(dockerCmd, "pull", "-q", goTestImage), "go test's image")
	assert.Equal(t, 1, fake.ran(dockerCmd, "pull", "-q", goImage), "gofmt's image")
	assert.Equal(t, 1, fake.ran(dockerCmd, runCmd, rm, "-v", mount, "-w", appDir, "-v", "thehivemcp-gomod:/go/pkg/mod",
		"-v", "thehivemcp-gobuild:/root/.cache/go-build", "-v", "thehivemcp-golangci-cache:/root/.cache/golangci-lint",
		"-e", "GOOS=linux", "-e", "GOARCH=amd64", golangci),
		"golangci-lint ran once")
}

// hadolint reads its Dockerfile on stdin; a fragment's checkmake configuration is written first,
// readable by the image's user.
func TestRunFeedsStdinAndWritesConfiguration(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "deployment"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, dockerf), []byte("FROM scratch\n"), 0o600))

	fake := newFake()
	fake.answers["hadolint"] = answer{}

	steps := lint.Steps(lint.Classify([]string{dockerf, mkFile}), repo, false)
	rep := lint.Runner{Root: dir, Exec: fake.exec}.Run(t.Context(), steps)

	assert.Empty(t, rep.Failures)
	assert.Equal(t, "FROM scratch\n", fake.stdins["hadolint"])

	info, err := os.Stat(filepath.Join(dir, lint.FragmentConfig))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	missing := lint.Runner{Root: t.TempDir(), Exec: fake.exec}.Run(t.Context(), steps)
	require.Len(t, missing.Failures, 1, "a Dockerfile that cannot be read is a finding")
	assert.Contains(t, missing.Failures[0].Output, "read deployment/Dockerfile")
}
