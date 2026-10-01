// Command harness runs the gates of gates/registry.yaml: the checks a change must pass, selected
// by the paths it changed. Run it through bin/harness, which builds it in the pinned Go image.
//
//	harness run --cadence stop|commit|pr|ci|nightly [--base REF | --staged | --all] [--gate ID]... [--force]
//	harness select [--base REF | --staged | --all]
//	harness list [--cadence C]
//	harness check structure|public-safety|docs|go-source|branch|spec|changelog
//	harness docs [--check]
//	harness hook pre-tool-use|post-tool-use|stop|subagent-stop|session-start|pre-compact  (JSON on stdin)
//	harness push [--force]
//	harness canaries [--id ID]... [--integration]
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gatesdoc"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
)

const (
	registryPath = "gates/registry.yaml"
	limitsPath   = "gates/limits.yaml"
	exitUsage    = 2
	// exitIncomplete: no gate failed, but at least one could not run, so the run is not a pass.
	exitIncomplete = 3
	dockerProbe    = 10 * time.Second
	filePerm       = 0o644
)

var (
	// errFailed means a gate failed; the results are already printed.
	errFailed = errors.New("failed")
	// errIncomplete means a gate was UNAVAILABLE: a stated limit, never a pass.
	errIncomplete = errors.New("incomplete")
)

func main() {
	// Ctrl-C, or a hook's timeout, cancels the run: each running gate's process group is stopped.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := dispatch(ctx, os.Args[1:], os.Stdin, os.Stdout)

	stop()

	var usage usageError
	switch {
	case err == nil:
	case errors.Is(err, errFailed):
		os.Exit(1)
	case errors.Is(err, errIncomplete):
		_, _ = fmt.Fprintln(os.Stderr, "harness: a gate could not run (UNAVAILABLE above), so this run is not a pass")

		os.Exit(exitIncomplete)
	case errors.As(err, &usage):
		_, _ = fmt.Fprintln(os.Stderr, "harness:", err)

		os.Exit(exitUsage)
	default:
		_, _ = fmt.Fprintln(os.Stderr, "harness:", err)

		os.Exit(1)
	}
}

type usageError struct{ msg string }

func (u usageError) Error() string { return u.msg }

// env is what every subcommand needs: the repository root, its registry and its limits.
type env struct {
	root   string
	reg    *registry.Registry
	limits checks.Limits
	// staged makes the built-in checks read the index, for a commit, instead of the working tree.
	staged bool
	// changed are the paths the change touches since base, for the checks that judge a change.
	changed []string
	base    string
}

// load reads the registry and the limits: from the index when staged (the pre-commit hook), so
// that what the commit carries decides which gates run, what they run and what they hold to,
// whatever the working tree holds.
func load(ctx context.Context, staged bool) (env, error) {
	wd, err := os.Getwd()
	if err != nil {
		return env{}, fmt.Errorf("working directory: %w", err)
	}

	root, err := gitx.Root(ctx, wd)
	if err != nil {
		return env{}, fmt.Errorf("find the repository: %w", err)
	}

	read := func(p string) ([]byte, error) {
		return os.ReadFile(filepath.Join(root, p)) // #nosec G304 -- the repository's own registry and limits
	}
	if staged {
		read = func(p string) ([]byte, error) { return gitx.Indexed(ctx, root, p) }
	}

	data, err := read(registryPath)
	if err != nil {
		return env{}, fmt.Errorf("load the gates: %w", err)
	}

	reg, err := registry.Parse(data, registryPath)
	if err != nil {
		return env{}, fmt.Errorf("load the gates: %w", err)
	}

	data, err = read(limitsPath)
	if err != nil {
		return env{}, fmt.Errorf("load the limits: %w", err)
	}

	limits, err := checks.ParseLimits(data, limitsPath)
	if err != nil {
		return env{}, fmt.Errorf("load the limits: %w", err)
	}

	for _, g := range reg.Gates {
		if g.Builtin != "" && !isBuiltin(g.Builtin) {
			return env{}, fmt.Errorf("gate %q: unknown builtin %q", g.ID, g.Builtin)
		}
	}

	return env{root: root, reg: reg, limits: limits, staged: staged}, nil
}

// stagedRun reports whether the command judges the index: run or select with --staged.
func stagedRun(args []string) bool {
	if len(args) == 0 || (args[0] != cmdRun && args[0] != cmdSelect) {
		return false
	}

	return slices.Contains(args[1:], "--staged") || slices.Contains(args[1:], "-staged")
}

func dispatch(ctx context.Context, args []string, stdin io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return usageError{"usage: harness run|select|list|check|docs|hook|push|canaries (see cmd/harness/main.go)"}
	}

	e, err := load(ctx, stagedRun(args))
	if err != nil {
		return err
	}

	switch args[0] {
	case cmdRun:
		return e.run(ctx, args[1:], out)
	case cmdSelect:
		return e.selectCmd(ctx, args[1:], out)
	case "list":
		return e.list(args[1:], out)
	case "check":
		return e.check(ctx, args[1:], out)
	case "docs":
		return e.docs(args[1:], out)
	case "hook":
		return e.hook(ctx, args[1:], stdin, out)
	case "push":
		return e.push(ctx, args[1:], out)
	case "canaries":
		return e.canaries(ctx, args[1:], out)
	default:
		return usageError{fmt.Sprintf("unknown command %q", args[0])}
	}
}

// The commands that select gates from the changed paths.
const (
	cmdRun    = "run"
	cmdSelect = "select"
)

// The checks built into the harness, named by a gate's builtin field.
const (
	builtinStructure = "structure"
	builtinSafety    = "public-safety"
	builtinDocs      = "docs"
	builtinGoSource  = "go-source"
	builtinBranch    = "branch"
	builtinSpec      = "spec"
	builtinChangelog = "changelog"
)

func isBuiltin(name string) bool {
	switch name {
	case builtinStructure, builtinSafety, builtinDocs, builtinGoSource, builtinBranch, builtinSpec, builtinChangelog:
		return true
	}

	return false
}

// tree is what the built-in checks read: the index for a commit (exactly what it carries), else
// the working tree with its untracked files.
func (e env) tree(ctx context.Context) (checks.Tree, error) {
	if e.staged {
		files, err := gitx.Index(ctx, e.root)
		if err != nil {
			return nil, fmt.Errorf("read the index: %w", err)
		}

		return checks.MemTree(files), nil
	}

	files, err := gitx.Files(ctx, e.root)
	if err != nil {
		return nil, fmt.Errorf("list the files: %w", err)
	}

	return checks.DirTree{Root: e.root, Names: files}, nil
}

// builtin runs a check built into the harness over every file of the tree.
func (e env) builtin(ctx context.Context, name string) ([]string, error) {
	tree, err := e.tree(ctx)
	if err != nil {
		return nil, err
	}

	var found []checks.Finding

	switch name {
	case builtinStructure:
		found, err = checks.Structure(tree, e.limits.Structure)
	case builtinSafety:
		found, err = checks.PublicSafety(tree, e.limits.PublicSafety)
	case builtinDocs:
		found, err = checks.Docs(tree, e.reg)
	case builtinGoSource:
		found, err = checks.GoSource(tree)
	case builtinBranch:
		found = onBranch(gitx.Branch(ctx, e.root))
	case builtinSpec:
		found, err = checks.SpecTraces(tree, reg)
	case builtinChangelog:
		found = checks.Changelog(e.changed, e.base == "" || gitx.HasPath(ctx, e.root, e.base, checks.SurfaceSummary))
	default:
		return nil, fmt.Errorf("unknown builtin %q", name)
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	lines := make([]string, len(found))
	for i, f := range found {
		lines[i] = f.String()
	}

	return lines, nil
}

// onBranch is the branch check: work is committed on a branch, and main changes only when a
// person merges a pull request.
func onBranch(branch string) []checks.Finding {
	if branch == "main" {
		return []checks.Finding{{Msg: "this commit is on main: commit on a branch (<type>/DL-<n>-<name>) and open a pull request"}}
	}

	return nil
}

func dockerUp(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, dockerProbe)
	defer cancel()

	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

func (e env) check(ctx context.Context, args []string, out io.Writer) error {
	if len(args) != 1 || !isBuiltin(args[0]) {
		return usageError{"usage: harness check structure|public-safety|docs|go-source|branch|spec|changelog"}
	}

	lines, err := e.builtin(ctx, args[0])
	if err != nil {
		return err
	}

	for _, l := range lines {
		_, _ = fmt.Fprintln(out, l)
	}

	if len(lines) > 0 {
		return errFailed
	}

	_, _ = fmt.Fprintf(out, "%s: clean\n", args[0])

	return nil
}

func (e env) docs(args []string, out io.Writer) error {
	page := gatesdoc.Render(e.reg)

	p := filepath.Join(e.root, gatesdoc.Path)
	if len(args) == 1 && args[0] == "--check" {
		current, err := os.ReadFile(p) // #nosec G304 -- the generated page of this repository
		if err != nil || string(current) != page {
			_, _ = fmt.Fprintf(out, "%s is stale: run make harness-docs\n", gatesdoc.Path)
			return errFailed
		}

		return nil
	}

	err := os.WriteFile(p, []byte(page), filePerm)
	if err != nil {
		return fmt.Errorf("write %s: %w", gatesdoc.Path, err)
	}

	_, _ = fmt.Fprintf(out, "wrote %s\n", gatesdoc.Path)

	return nil
}

func (e env) list(args []string, out io.Writer) error {
	cadence := ""
	if len(args) == 2 && args[0] == "--cadence" {
		cadence = args[len(args)-1]
	}

	for _, g := range e.reg.Gates {
		if cadence == "" || g.RunsAt(cadence) {
			_, _ = fmt.Fprintf(out, "%-14s %-30s %s\n", g.ID, strings.Join(g.Cadence, ","), strings.Join(g.Suites, ","))
		}
	}

	return nil
}
