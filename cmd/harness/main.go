// Command harness runs the gates of gates/registry.yaml: the checks a change must pass, selected
// by the paths it changed. Run it through bin/harness, which builds it in the pinned Go image.
//
//	harness run --cadence stop|commit|pr|ci|nightly [--base REF | --staged | --all] [--gate ID]... [--force]
//	harness select [--base REF | --staged | --all]
//	harness list [--cadence C]
//	harness check structure|public-safety|docs|go-source|branch|spec|changelog|integrity
//	harness docs [--check]
//	harness hook pre-tool-use|post-tool-use|stop|subagent-stop|session-start|pre-compact  (JSON on stdin)
//	harness pr [--push=false] [--force] [--person]  (make pr: the pr gates at HEAD, the evidence, the push)
//	harness push [--force] [--person]  (a person's: a Claude session pushes through make pr)
//	harness evidence [--markdown] [--base REF]
//	harness review-effort [--base REF]
//	harness canaries [--id ID]... [--integration]
//	harness structure [--top N] [--json]  (the largest files and fullest directories against the limits)
//	harness lint [--all | --staged | --changed [COMMIT] | PATH...] [--check | --fix]  (the pinned linters; check by default)
//	harness fmt [--all | --staged | --changed [COMMIT] | PATH...] [--check | --fix]  (the pinned formatters; fix by default)
//	harness package tarballs|mcpb [--version V] [--build DIR] [--dist DIR] [--name N] [--targets "T ..." | --host]
//	harness sign-windows BINARY...  (SIGNING_PROVIDER: none, signpath or azure)
//	harness integration  (make test-integration: the stack up and reset, then the suite)
//	harness unseal --ticket KEY  (the end of a duel: the Tester's sealed tests move into the code)
//	harness mutation [--base REF | --all]  (gremlins on the changed packages, or every floored one, against gates/mutation.yaml)
//	harness bench [--base REF]  (the benchmarks at the merge base and at HEAD, against gates/bench.yaml)
//	harness triage --runs DIR --run-url URL [--expect SHARD,...] [--dry-run]  (the night's failures as issues labelled nightly)
//	harness next  (make next: a blocked agent, then the open nightly findings, the oldest first)
//	harness eval-smoke [--eval DIR] [--out FILE] [--previous FILE]  (TheHiveMCPEval's suite on gates/eval.yaml's model, held to its cap and the previous night)
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
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	registryPath = "gates/registry.yaml"
	limitsPath   = "gates/limits.yaml"
	exitUsage    = 2
	// exitIncomplete: no gate failed, but at least one could not run, so the run is not a pass.
	exitIncomplete = 3
	dockerProbe    = 10 * time.Second
	filePerm       = 0o644
	dirPerm        = 0o750
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

	code, msg := exitFor(err)
	if msg != "" {
		_, _ = fmt.Fprintln(os.Stderr, msg)
	}

	if code != 0 {
		os.Exit(code)
	}
}

// errCannotRun means the command cannot run here: its gate is UNAVAILABLE, never a pass.
var errCannotRun = errors.New("cannot run here")

// exitFor is the exit code of a command's error, and what to say on stderr: a command that cannot
// run here exits 75, which the runner reads as UNAVAILABLE.
func exitFor(err error) (int, string) {
	var usage usageError

	switch {
	case err == nil:
		return 0, ""
	case errors.Is(err, errFailed):
		return 1, ""
	case errors.Is(err, errCannotRun):
		return runner.ExitUnavailable, "harness: " + err.Error()
	case errors.Is(err, errIncomplete):
		return exitIncomplete, "harness: a gate could not run (UNAVAILABLE above), so this run is not a pass"
	case errors.As(err, &usage):
		return exitUsage, "harness: " + err.Error()
	default:
		return 1, "harness: " + err.Error()
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
	root, err := repositoryRoot(ctx)
	if err != nil {
		return env{}, err
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

// repositoryRoot is the top of the working tree the harness runs in.
func repositoryRoot(ctx context.Context) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}

	root, err := gitx.Root(ctx, wd)
	if err != nil {
		return "", fmt.Errorf("find the repository: %w", err)
	}

	return root, nil
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
		return usageError{"usage: harness run|select|list|check|docs|hook|pr|push|evidence|review-effort|canaries|structure|lint|fmt|package|sign-windows|integration|bench|mutation|triage|next|eval-smoke|unseal (see cmd/harness/main.go)"}
	}

	if command, ok := standalone[args[0]]; ok {
		// These read the repository alone: a registry or limits file half edited, or staged in
		// part, cannot stop a lint, a commit, a test run or a release.
		root, err := repositoryRoot(ctx)
		if err != nil {
			return err
		}

		return command(env{root: root}, ctx, args[1:], out)
	}

	e, err := load(ctx, stagedRun(args))
	if err != nil {
		return err
	}

	commands := map[string]func(context.Context, []string, io.Writer) error{
		cmdRun:           e.run,
		cmdSelect:        e.selectCmd,
		"list":           func(_ context.Context, a []string, w io.Writer) error { return e.list(a, w) },
		"check":          e.check,
		"docs":           func(_ context.Context, a []string, w io.Writer) error { return e.docs(a, w) },
		"hook":           func(ctx context.Context, a []string, w io.Writer) error { return e.hook(ctx, a, stdin, w) },
		"push":           e.push,
		"canaries":       e.canaries,
		"pr":             e.pr,
		"evidence":       e.showEvidence,
		builtinStructure: e.structure,
		"review-effort":  e.reviewEffort,
		"bench":          e.bench,
		"mutation":       e.mutate,
		"triage":         e.triageNight,
		"debt":           e.debt,
		"eval-smoke":     e.evalSmoke,
		"next":           func(ctx context.Context, _ []string, w io.Writer) error { return e.next(ctx, w) },
		"unseal":         func(_ context.Context, a []string, w io.Writer) error { return e.unseal(a, w) },
	}

	command, ok := commands[args[0]]
	if !ok {
		return usageError{fmt.Sprintf("unknown command %q", args[0])}
	}

	return command(ctx, args[1:], out)
}

// standalone are the commands that need the repository only, not its registry or limits.
var standalone = map[string]func(env, context.Context, []string, io.Writer) error{
	cmdLint:        env.lint,
	cmdFmt:         env.fmtCmd,
	cmdPackage:     env.packageCmd,
	cmdSign:        env.signWindows,
	cmdIntegration: env.integration,
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
	builtinIntegrity = "integrity"
)

func isBuiltin(name string) bool {
	switch name {
	case builtinStructure, builtinSafety, builtinDocs, builtinGoSource, builtinBranch, builtinSpec, builtinChangelog, builtinIntegrity:
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
		found, err = checks.SpecTraces(tree, e.reg)
	case builtinChangelog:
		found, err = e.changelog(ctx)
	case builtinIntegrity:
		found, err = e.integrity(ctx, tree, e.limits.Integrity)
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

// integrity compares the changed tests with those at the merge base, where the changed paths are
// counted from. A run with no base to compare with (--all, a push to main, no origin/main) has no
// test to compare: its gate is SKIPPED, not passed.
func (e env) integrity(ctx context.Context, tree checks.Tree, limits checks.IntegrityLimits) ([]checks.Finding, error) {
	if e.base == "" {
		return nil, fmt.Errorf("%w: no base to compare with, so no changed test to compare", runner.ErrSkipped)
	}

	base := gitx.MergeBase(ctx, e.root, e.base)
	if base == "" {
		base = e.base
	}

	before := func(p string) ([]byte, bool) { return gitx.At(ctx, e.root, base, p) }

	found, err := checks.Integrity(tree, e.changed, before, limits)
	if err != nil {
		return nil, fmt.Errorf("compare the tests with %s: %w", e.base, err)
	}

	return found, nil
}

// onBranch is the branch check: work is committed on a branch, and main changes only when a
// person merges a pull request.
func onBranch(branch string) []checks.Finding {
	if branch == mainBranch {
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
	since := len(args) > 0 && (args[0] == builtinChangelog || args[0] == builtinIntegrity)
	if len(args) == 0 || !isBuiltin(args[0]) || (len(args) > 1 && !since) {
		return usageError{"usage: harness check structure|public-safety|docs|go-source|branch|spec|changelog|integrity [--base REF]"}
	}

	if since {
		var err error

		e, err = e.changeSet(ctx, args[1:])
		if err != nil {
			return err
		}
	}

	lines, err := e.builtin(ctx, args[0])
	if errors.Is(err, runner.ErrSkipped) {
		_, _ = fmt.Fprintf(out, "%s: not judged: %v\n", args[0], err)
		return errIncomplete
	}

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
