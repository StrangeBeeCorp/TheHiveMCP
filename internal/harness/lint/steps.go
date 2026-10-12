package lint

import (
	"path"
	"strings"
)

// Fix says whether a step rewrites files, and under which heading its changes are reported.
type Fix int

// The kinds of step: a check reports, a fix rewrites (its exit and output are not findings).
const (
	Check Fix = iota
	// Format is a formatter's rewrite.
	Format
	// LintFix is a linter's safe fix.
	LintFix
)

// Step is one run of one tool: its docker command line, and how its result is read.
type Step struct {
	// Name heads the step's findings.
	Name  string
	Image string
	Argv  []string
	// Stdin is a file, relative to the root, fed to the tool on stdin.
	Stdin string
	// Lists is a tool that prints the files it would rewrite: any output is a finding.
	Lists bool
	Fix   Fix
	// Before is a file the runner writes first, relative to the root.
	Before *File
}

// File is a file a step needs written.
type File struct {
	Path    string
	Content []byte
}

// Repo is what the steps need to know of the checkout.
type Repo struct {
	// Root is the absolute path docker mounts.
	Root   string
	Images Images
	// HadolintConfig is whether .hadolint.yaml exists; CheckmakeINI is checkmake.ini, nil when
	// there is none.
	HadolintConfig bool
	CheckmakeINI   []byte
}

const (
	app       = "/app"
	dockerCmd = "docker"
	gofmtCmd  = "gofmt"
	// FragmentConfig is the checkmake configuration of a Makefile fragment (*.mk): the repository's
	// rules without minphony, since the all, clean and test targets it asks for are the including
	// Makefile's.
	FragmentConfig = ".cache/harness/checkmake-fragment.ini"
	yamllintRules  = "{extends: relaxed, rules: {line-length: disable, document-start: disable, comments: disable, " +
		"comments-indentation: disable, empty-lines: disable, trailing-spaces: disable}}"
)

// The named volumes of the Makefile's Go targets: the module and build caches, and golangci-lint's.
// golangci-lint type-checks the code as CI builds it, linux/amd64: on another machine a constant
// that depends on the platform (runtime.GOARCH in a test's map key) reads otherwise.
var (
	goCaches       = []string{"-v", "thehivemcp-gomod:/go/pkg/mod", "-v", "thehivemcp-gobuild:/root/.cache/go-build"}
	golangciCaches = append(goCaches[:len(goCaches):len(goCaches)],
		"-v", "thehivemcp-golangci-cache:/root/.cache/golangci-lint", "-e", "GOOS=linux", "-e", "GOARCH=amd64")
)

// docker is a docker run of image with the root mounted at /app as the working directory.
func (r Repo) docker(image string, opts []string, args ...string) []string {
	argv := []string{dockerCmd, "run", "--rm", "-v", r.Root + ":" + app, "-w", app}
	argv = append(argv, opts...)
	argv = append(argv, image)

	return append(argv, args...)
}

func (r Repo) step(name, image string, opts []string, args ...string) Step {
	return Step{Name: name, Image: image, Argv: r.docker(image, opts, args...)}
}

// inApp is each path as the container sees it.
func inApp(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = path.Join(app, p)
	}

	return out
}

func with(args []string, paths ...string) []string {
	return append(args[:len(args):len(args)], paths...)
}

// FormatSteps are bin/harness fmt's: with fix, gofmt and golangci-lint fmt, shfmt, markdownlint
// --fix and prettier rewrite the files; without, gofmt, shfmt and prettier list the files they
// would rewrite.
func FormatSteps(f Files, r Repo, fix bool) []Step {
	var steps []Step

	prettier := []string{"npx", "--yes", "prettier@" + prettierVersion}
	npm := []string{"-e", "npm_config_update_notifier=false"}
	shfmt := []string{"-i", "2", "-ci", "-l"}

	if !fix {
		if len(f.Go) > 0 {
			steps = append(steps, listing(r.step("gofmt (would reformat)", r.Images.Go, nil, with([]string{gofmtCmd, "-l"}, inApp(f.Go)...)...)))
		}

		if len(f.Shell) > 0 {
			steps = append(steps, listing(r.step("shfmt (would reformat)", shfmtImage, nil, with(shfmt, f.Shell...)...)))
		}

		if len(f.Markdown) > 0 {
			steps = append(steps, listing(r.step("prettier (would reformat)", prettierImage, npm, with(prettier, with([]string{"--list-different"}, inApp(f.Markdown)...)...)...)))
		}

		return steps
	}

	if len(f.Go) > 0 {
		steps = append(steps,
			fixing(r.step(gofmtCmd, r.Images.Go, nil, with([]string{gofmtCmd, "-l", "-w"}, inApp(f.Go)...)...), Format),
			fixing(r.step("golangci-lint fmt", golangciImage, golangciCaches, "golangci-lint", "fmt", "./..."), Format))
	}

	if len(f.Shell) > 0 {
		steps = append(steps, fixing(r.step("shfmt", shfmtImage, nil, with(shfmt, with([]string{"-w"}, f.Shell...)...)...), Format))
	}

	if len(f.Markdown) > 0 {
		steps = append(steps,
			fixing(r.step("markdownlint-cli2 --fix", markdownImage, nil, with([]string{"markdownlint-cli2", "--fix"}, inApp(f.Markdown)...)...), Format),
			fixing(r.step("prettier", prettierImage, npm, with(prettier, with([]string{"--write"}, inApp(f.Markdown)...)...)...), Format))
	}

	return steps
}

// Steps are bin/harness lint's. With fix, the formatters and golangci-lint's safe fixes run
// first; then golangci-lint, go test -short, shellcheck, lychee (local links only, so an outage
// elsewhere cannot fail a run), yamllint, hadolint, checkmake and actionlint check, and without
// fix gofmt, shfmt and markdownlint report what the formatters would change.
func Steps(f Files, r Repo, fix bool) []Step {
	var steps []Step

	shfmt := []string{"-i", "2", "-ci", "-l"}

	if fix {
		steps = append(steps, FormatSteps(f, r, true)...)
	}

	if len(f.Go) > 0 {
		if fix {
			steps = append(steps, fixing(r.step("golangci-lint run --fix", golangciImage, golangciCaches, "golangci-lint", "run", "--fix", "./..."), LintFix))
		} else {
			steps = append(steps, listing(r.step("gofmt -l (needs formatting; run make fmt)", r.Images.Go, nil, with([]string{gofmtCmd, "-l"}, inApp(f.Go)...)...)))
		}

		steps = append(steps,
			r.step("golangci-lint", golangciImage, golangciCaches, "golangci-lint", "run", "./..."),
			r.step("go test -short", r.Images.GoTest, with(goCaches, "-e", "CGO_ENABLED=0"), "go", "test", "-short", "./..."))
	}

	if len(f.Shell) > 0 {
		steps = append(steps, r.step("shellcheck", shellcheckImage, nil, f.Shell...))
		if !fix {
			steps = append(steps, listing(r.step("shfmt (needs formatting; run make fmt)", shfmtImage, nil, with(shfmt, f.Shell...)...)))
		}
	}

	if len(f.Markdown) > 0 {
		if !fix {
			steps = append(steps, r.step("markdownlint-cli2", markdownImage, nil, with([]string{"markdownlint-cli2"}, inApp(f.Markdown)...)...))
		}

		steps = append(steps, r.step("lychee", lycheeImage, []string{"--init"}, with([]string{"--offline"}, f.Markdown...)...))
	}

	if len(f.YAML) > 0 {
		steps = append(steps, r.step("yamllint", yamllintImage, nil, with([]string{"-d", yamllintRules}, f.YAML...)...))
	}

	for _, df := range f.Dockerfiles {
		steps = append(steps, r.hadolint(df))
	}

	steps = append(steps, r.checkmake(f.Makefiles)...)

	if len(f.Workflows) > 0 {
		steps = append(steps, r.step("actionlint", actionlintImage, nil, with([]string{"-no-color"}, f.Workflows...)...))
	}

	return steps
}

// hadolint reads one Dockerfile on stdin, with .hadolint.yaml when the repository has one.
func (r Repo) hadolint(dockerfile string) Step {
	argv := []string{dockerCmd, "run", "--rm", "-i"}
	args := []string{"hadolint", "-"}

	if r.HadolintConfig {
		argv = append(argv, "-v", r.Root+"/.hadolint.yaml:/.hadolint.yaml:ro")
		args = []string{"hadolint", "--config", "/.hadolint.yaml", "-"}
	}

	return Step{
		Name: "hadolint (" + dockerfile + ")", Image: hadolintImage,
		Argv: append(append(argv, hadolintImage), args...), Stdin: dockerfile,
	}
}

// checkmake checks the Makefiles with checkmake.ini, and the fragments (*.mk) with the same rules
// but minphony.
func (r Repo) checkmake(makefiles []string) []Step {
	var whole, parts []string

	for _, m := range makefiles {
		if strings.HasSuffix(m, ".mk") {
			parts = append(parts, m)
		} else {
			whole = append(whole, m)
		}
	}

	entry := []string{"--entrypoint", "/checkmake"}

	var steps []Step

	if len(whole) > 0 {
		opts, args := entry, inApp(whole)
		if r.CheckmakeINI != nil {
			opts = with(entry, "-v", r.Root+"/checkmake.ini:/checkmake.ini:ro")
			args = with([]string{"--config=/checkmake.ini"}, args...)
		}

		steps = append(steps, r.step("checkmake", checkmakeImage, opts, args...))
	}

	if len(parts) > 0 {
		s := r.step("checkmake (fragments)", checkmakeImage, with(entry, "-v", r.Root+"/"+FragmentConfig+":/checkmake.ini:ro"),
			with([]string{"--config=/checkmake.ini"}, inApp(parts)...)...)
		s.Before = &File{Path: FragmentConfig, Content: append(append([]byte{}, r.CheckmakeINI...), "\n[minphony]\ndisabled = true\n"...)}
		steps = append(steps, s)
	}

	return steps
}

func listing(s Step) Step {
	s.Lists = true
	return s
}

func fixing(s Step, kind Fix) Step {
	s.Fix = kind
	return s
}
