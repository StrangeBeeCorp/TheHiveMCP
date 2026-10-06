// Command load runs one run of gates/load.yaml against a TheHive stub: it builds the server, runs
// it as its HTTP users do, drives it with the run's clients, and holds the result to the run's
// budget (the load and soak gates). The result is compared with baselines/load/<run>.json, and
// --write-baseline records it there, when a person sets the budgets.
//
//	go run ./cmd/load --run stub|soak [--write-baseline]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/load"
)

// errOverBudget means the run exceeded its budget; the problems are printed.
var errOverBudget = errors.New("over budget")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)

	stop()

	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "load:", err)

		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("load", flag.ContinueOnError)
	name := fs.String("run", "", "the run of "+load.BudgetsPath+" to make")
	write := fs.Bool("write-baseline", false, "record the result as the run's baseline, under "+load.BaselineDir)

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("usage: %w", err)
	}

	budgets, err := load.ReadBudgets(load.BudgetsPath)
	if err != nil {
		return fmt.Errorf("budgets: %w", err)
	}

	r, err := budgets.Run(*name)
	if err != nil {
		return fmt.Errorf("--run: %w", err)
	}

	dir, err := os.MkdirTemp("", "load-")
	if err != nil {
		return fmt.Errorf("build directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	bin, err := load.Build(ctx, ".", dir)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	stub, target := load.NewStub()
	defer stub.Close()

	result, err := load.Measure(ctx, *name, r.Config, bin, target, out)
	if err != nil {
		return fmt.Errorf("measure: %w", err)
	}

	_, _ = fmt.Fprintln(out, result.Summary())

	return report(result, r.Budget, *write, out)
}

// report compares the result with its baseline, records it as the baseline if asked, and holds
// it to its budget.
func report(result load.Result, budget load.Budget, write bool, out io.Writer) error {
	path := load.Baseline(".", result.Run)

	base, err := load.ReadResult(path)

	switch {
	case err == nil:
		_, _ = fmt.Fprintln(out, load.Compare(result, base))
	case errors.Is(err, fs.ErrNotExist):
		_, _ = fmt.Fprintf(out, "%s has no baseline yet (%s)\n", result.Run, path)
	default:
		return fmt.Errorf("baseline: %w", err)
	}

	if write {
		err = result.Write(path)
		if err != nil {
			return fmt.Errorf("baseline: %w", err)
		}

		_, _ = fmt.Fprintln(out, "baseline written:", path)
	}

	problems := budget.Check(result)
	for _, p := range problems {
		_, _ = fmt.Fprintln(out, p)
	}

	if len(problems) > 0 {
		return fmt.Errorf("%w: %d problem(s), see %s", errOverBudget, len(problems), load.BudgetsPath)
	}

	_, _ = fmt.Fprintf(out, "%s within its budget\n", result.Run)

	return nil
}
