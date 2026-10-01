package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// defaultBase is what a branch is compared with when no base is given.
const defaultBase = "origin/main"

// scopeFlags choose the changed paths that select the suites.
type scopeFlags struct {
	base   string
	staged bool
	all    bool
}

func (s *scopeFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&s.base, "base", os.Getenv("HARNESS_BASE"), "compare with the merge base of this ref and HEAD (default origin/main; $HARNESS_BASE)")
	fs.BoolVar(&s.staged, "staged", false, "select from the staged files only (the pre-commit hook)")
	fs.BoolVar(&s.all, "all", false, "select every suite")
}

// selection works out the suites the change selects. In CI, an empty $HARNESS_BASE (a push to
// main) selects everything.
func (s *scopeFlags) selection(ctx context.Context, e env) (registry.Selection, error) {
	switch {
	case s.all:
		return registry.SelectAll("asked for every gate"), nil
	case s.staged:
		paths, err := gitx.Staged(ctx, e.root)
		if err != nil {
			return registry.Selection{}, fmt.Errorf("list the staged files: %w", err)
		}

		sel := e.reg.Select(paths)
		sel.Base = "HEAD"

		return sel, nil
	}

	base := s.base
	if base == "" && os.Getenv("CI") != "" {
		return registry.SelectAll("CI without a base: a push, not a pull request"), nil
	}

	if base == "" {
		base = defaultBase
	}

	if !gitx.HasRef(ctx, e.root, base) {
		return registry.SelectAll(fmt.Sprintf("no %s to compare with", base)), nil
	}

	paths, err := gitx.ChangedSince(ctx, e.root, base)
	if err != nil {
		return registry.Selection{}, fmt.Errorf("list the changes since %s: %w", base, err)
	}

	sel := e.reg.Select(paths)
	sel.Base = base

	return sel, nil
}

type gateList []string

func (g *gateList) String() string { return strings.Join(*g, ",") }

func (g *gateList) Set(v string) error {
	*g = append(*g, strings.Split(v, ",")...)
	return nil
}

func (e env) selectCmd(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("select", flag.ContinueOnError)

	var scope scopeFlags
	scope.register(fs)

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	sel, err := scope.selection(ctx, e)
	if err != nil {
		return err
	}

	if sel.All {
		_, _ = fmt.Fprintf(out, "every suite: %s\n", sel.Reason)
		return nil
	}

	for _, s := range sel.Names() {
		_, _ = fmt.Fprintf(out, "%-12s %s\n", s, strings.Join(sel.Suites[s], " "))
	}

	return nil
}

func (e env) run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)

	var scope scopeFlags
	scope.register(fs)
	cadence := fs.String("cadence", "", "the cadence to run: "+strings.Join(registry.Cadences, ", "))
	force := fs.Bool("force", false, "run the named gates even when the change does not select them")
	stream := fs.Bool("stream", os.Getenv("CI") != "", "copy each command's output as it runs (default in CI)")

	var only gateList
	fs.Var(&only, "gate", "run only this gate (repeatable, or comma-separated)")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	if !slices.Contains(registry.Cadences, *cadence) {
		return usageError{"--cadence must be one of " + strings.Join(registry.Cadences, ", ")}
	}

	gates, err := e.gatesFor(*cadence, only)
	if err != nil {
		return err
	}

	sel, err := scope.selection(ctx, e)
	if err != nil {
		return err
	}

	if scope.staged != e.staged {
		return usageError{"--staged takes no value"}
	}

	e.changed, e.base = sel.Paths, sel.Base

	return e.execute(ctx, *cadence, gates, sel, runFlags{force: *force && len(only) > 0, stream: *stream}, out)
}

// gatesFor is the gates of the cadence, restricted to the ones named.
func (e env) gatesFor(cadence string, only []string) ([]registry.Gate, error) {
	if len(only) == 0 {
		return e.reg.At(cadence), nil
	}

	gates := make([]registry.Gate, 0, len(only))
	for _, id := range only {
		g, ok := e.reg.Gate(id)
		if !ok {
			return nil, usageError{fmt.Sprintf("no gate %q in %s", id, registryPath)}
		}

		gates = append(gates, g)
	}

	return gates, nil
}

type runFlags struct {
	force  bool
	stream bool
}

func (e env) execute(ctx context.Context, cadence string, gates []registry.Gate, sel registry.Selection, f runFlags, out io.Writer) error {
	rec, err := e.runGates(ctx, cadence, gates, sel, f, out)
	if err != nil {
		return err
	}

	switch rec.Verdict {
	case runner.Fail:
		return errFailed
	case runner.Incomplete:
		return errIncomplete
	default:
		return nil
	}
}

// runGates runs the gates the selection selects, prints their results and records the run.
func (e env) runGates(ctx context.Context, cadence string, gates []registry.Gate, sel registry.Selection, f runFlags, out io.Writer) (runner.Record, error) {
	var run, skipped []registry.Gate

	for _, g := range gates {
		if f.force || sel.Selects(g) {
			run = append(run, g)
		} else {
			skipped = append(skipped, g)
		}
	}

	header := "suites: " + strings.Join(sel.Names(), ", ")
	if sel.All {
		header = "every suite (" + sel.Reason + ")"
	}

	_, _ = fmt.Fprintf(out, "harness · %s · %s · %d gate(s)\n", cadence, header, len(run))
	started := time.Now()
	logDir := filepath.Join(e.root, ".cache", "harness", "runs", cadence+"-"+started.Format("20060102-150405"))

	results, err := runner.Run(ctx, e.reg, run, runner.Options{
		Root: e.root, Cadence: cadence, LogDir: logDir, Stream: f.stream, Out: out,
		Builtin: e.builtin, DockerUp: dockerUp,
	})
	if err != nil {
		return runner.Record{}, fmt.Errorf("run the gates: %w", err)
	}

	for _, g := range skipped {
		results = append(results, runner.Result{Gate: g.ID, Verdict: runner.Skipped, Summary: "not selected by this change"})
	}

	runner.Print(out, results)

	rec := runner.Record{
		Cadence: cadence, Head: gitx.Head(ctx, e.root), Suites: sel.Names(), Reason: sel.Reason,
		Started: started, Finished: time.Now(), Verdict: runner.Overall(results), Results: results,
	}

	p, err := rec.Write(logDir)
	if err != nil {
		return runner.Record{}, fmt.Errorf("record the run: %w", err)
	}

	_, _ = fmt.Fprintf(out, "verdict %s · %s\n", rec.Verdict, p)

	return rec, nil
}
