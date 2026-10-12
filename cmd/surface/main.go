// Command surface checks what the server advertises to a model against baselines/surface/.
// Run it from the repository root, through make (make go CMD="run ./cmd/surface …").
//
//	surface --check     the surface gate: exit 1 when the server differs from the snapshot
//	surface --budgets   the tokens gate: exit 1 when gates/budgets.yaml is exceeded
//	surface --write     rewrite the snapshot (make surface-propose)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/surface"
)

const (
	budgetsPath = "gates/budgets.yaml"
	exitUsage   = 2
)

var errUsage = errors.New("give --check, --budgets or --write")

func main() {
	check := flag.Bool("check", false, "fail when the server differs from "+surface.Dir)
	budgets := flag.Bool("budgets", false, "fail when the surface exceeds "+budgetsPath)
	write := flag.Bool("write", false, "rewrite "+surface.Dir)

	flag.Parse()

	// The server logs while it registers its tools; the gate's output is the verdict alone.
	slog.SetDefault(slog.New(slog.DiscardHandler))

	failed, err := run(context.Background(), *check, *budgets, *write, os.Stdout)

	switch {
	case errors.Is(err, errUsage):
		_, _ = fmt.Fprintln(os.Stderr, "surface:", err)

		os.Exit(exitUsage)
	case err != nil:
		_, _ = fmt.Fprintln(os.Stderr, "surface:", err)

		os.Exit(1)
	case failed:
		os.Exit(1)
	}
}

func run(ctx context.Context, check, budgets, write bool, out io.Writer) (bool, error) {
	if !check && !budgets && !write {
		return false, errUsage
	}

	snap, err := surface.Take(ctx)
	if err != nil {
		return false, fmt.Errorf("read the surface: %w", err)
	}

	files, err := snap.Files()
	if err != nil {
		return false, fmt.Errorf("render the snapshot: %w", err)
	}

	_, _ = fmt.Fprintln(out, snap.Summary().Describe())

	dir := filepath.FromSlash(surface.Dir)
	failed := false

	if write {
		err = surface.Write(dir, files)
		if err != nil {
			return false, fmt.Errorf("write the snapshot: %w", err)
		}

		_, _ = fmt.Fprintf(out, "wrote %s (%d files)\n", surface.Dir, len(files))
	}

	if check {
		drift, err := drifted(dir, files, out)
		if err != nil {
			return false, err
		}

		failed = failed || drift
	}

	if budgets {
		over, err := overBudget(snap.Summary(), out)
		if err != nil {
			return false, err
		}

		failed = failed || over
	}

	return failed, nil
}

// drifted prints the files that differ from the committed snapshot.
func drifted(dir string, files map[string][]byte, out io.Writer) (bool, error) {
	drift, err := surface.Diff(dir, files)
	if err != nil {
		return false, fmt.Errorf("compare with the snapshot: %w", err)
	}

	for _, d := range drift {
		_, _ = fmt.Fprintln(out, surface.Dir+"/"+d)
	}

	if len(drift) > 0 {
		_, _ = fmt.Fprintln(out, "what the model reads changed: run make surface-propose, review the diff, "+
			"and add a CHANGELOG.md entry")
	}

	return len(drift) > 0, nil
}

// overBudget prints each budget of gates/budgets.yaml the surface exceeds.
func overBudget(sum surface.Summary, out io.Writer) (bool, error) {
	b, err := surface.LoadBudgets(budgetsPath)
	if err != nil {
		return false, fmt.Errorf("load the budgets: %w", err)
	}

	over := b.Over(sum)
	for _, o := range over {
		_, _ = fmt.Fprintln(out, o)
	}

	return len(over) > 0, nil
}
