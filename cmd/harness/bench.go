package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/bench"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
)

const (
	// benchPath holds the benchmarks' budgets.
	benchPath = "gates/bench.yaml"
	// benchLogLines is how much of a failed run's output the error shows.
	benchLogLines = 15
)

// errOverBudget means a benchmark is over one of its budgets.
var errOverBudget = errors.New("over budget")

// bench runs the benchmarks of gates/bench.yaml at the merge base and at HEAD, on this machine,
// and holds HEAD's to the budgets there (the bench gate). The base runs in a scratch worktree
// through this checkout's Makefile: the base's own may lack the go target.
func (e env) bench(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	base := fs.String("base", os.Getenv("HARNESS_BASE"), "compare with the merge base of this ref and HEAD (default origin/main; $HARNESS_BASE)")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	if *base == "" {
		*base = defaultBase
	}

	budgets, err := bench.Load(filepath.Join(e.root, benchPath))
	if err != nil {
		return fmt.Errorf("bench: %w", err)
	}

	head, err := e.runBench(ctx, e.root, budgets)
	if err != nil {
		return err
	}

	before := e.baseBench(ctx, *base, budgets, out)

	for _, row := range bench.Table(before, head) {
		_, _ = fmt.Fprintln(out, row)
	}

	problems := bench.Compare(before, head, budgets)
	for _, p := range problems {
		_, _ = fmt.Fprintln(out, p)
	}

	if len(problems) > 0 {
		return fmt.Errorf("%w: %d problem(s), see gates/bench.yaml", errOverBudget, len(problems))
	}

	_, _ = fmt.Fprintf(out, "%d benchmarks within their budgets\n", len(head))

	return nil
}

// baseBench runs the benchmarks at the merge base of base and HEAD. With no merge base, or when
// they do not run there, there is nothing to compare with: the absolute budgets still hold.
func (e env) baseBench(ctx context.Context, base string, budgets bench.Budgets, out io.Writer) map[string]bench.Result {
	mb := gitx.MergeBase(ctx, e.root, base)
	if mb == "" {
		_, _ = fmt.Fprintf(out, "no merge base with %s: only the absolute budgets hold\n", base)
		return nil
	}

	dir, remove, err := gitx.Scratch(ctx, e.root, mb)
	if err != nil {
		_, _ = fmt.Fprintf(out, "no scratch checkout of the base: %v: only the absolute budgets hold\n", err)
		return nil
	}
	defer remove()

	results, err := e.runBench(ctx, dir, budgets)
	if err != nil {
		_, _ = fmt.Fprintf(out, "the benchmarks do not run at the base: %v: only the absolute budgets hold\n", err)
		return nil
	}

	if len(results) == 0 {
		_, _ = fmt.Fprintf(out, "the base %.7s has none of these benchmarks: only the absolute budgets hold\n", mb)
	}

	return results
}

// runBench runs the benchmarks in dir, in the pinned Go image, through this checkout's Makefile.
func (e env) runBench(ctx context.Context, dir string, b bench.Budgets) (map[string]bench.Result, error) {
	cmd := exec.CommandContext(ctx, "make", "-f", filepath.Join(e.root, "Makefile"), "go", "CMD="+strings.Join(b.Args(), " ")) // #nosec G204 -- this checkout's Makefile; the arguments come from the reviewed budgets file
	cmd.Dir = dir

	var output bytes.Buffer

	cmd.Stdout, cmd.Stderr = &output, &output

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("benchmarks in %s: %w\n%s", dir, err, lastLines(output.String(), benchLogLines))
	}

	return bench.Parse(output.String()), nil
}
