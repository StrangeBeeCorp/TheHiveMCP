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

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/mutation"
)

// mutationPath holds the mutation floors and gremlins' pinned version.
const mutationPath = "gates/mutation.yaml"

// errMutation means a package's tests let too many mutants live.
var errMutation = errors.New("under the mutation floor")

// mutate runs gremlins on the packages the change touches that have a floor in
// gates/mutation.yaml, in the pinned Go image, and holds each to its floor (the mutation gate).
func (e env) mutate(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("mutation", flag.ContinueOnError)
	base := fs.String("base", os.Getenv("HARNESS_BASE"), "the change is what differs from the merge base of this ref and HEAD (default origin/main; $HARNESS_BASE)")

	err := fs.Parse(args)
	if err != nil {
		return usageError{err.Error()}
	}

	if *base == "" {
		*base = defaultBase
	}

	config, err := mutation.Load(filepath.Join(e.root, mutationPath))
	if err != nil {
		return fmt.Errorf("mutation: %w", err)
	}

	changed, err := gitx.ChangedSince(ctx, e.root, *base)
	if err != nil {
		return fmt.Errorf("list what changed since %s: %w", *base, err)
	}

	floored, unfloored := config.Packages(changed)
	if len(unfloored) > 0 {
		_, _ = fmt.Fprintf(out, "not mutated, no floor in %s: %s\n", mutationPath, strings.Join(unfloored, ", "))
	}

	failed, unmeasured := 0, 0

	for _, pkg := range floored {
		report, err := e.gremlins(ctx, config, pkg)
		if err != nil {
			return err
		}

		line, ok := config.Check(pkg, report)
		_, _ = fmt.Fprintln(out, line)

		switch {
		case ok:
		case report.Unmeasured():
			unmeasured++
		default:
			_, _ = fmt.Fprintf(out, "  gremlins' report: %s\n", reportPath(pkg))
			failed++
		}
	}

	// A package under its floor fails the gate; one whose mutants timed out measured nothing, so
	// the gate could not run, never a pass.
	if failed > 0 {
		return fmt.Errorf("%w: %d package(s)", errMutation, failed)
	}

	if unmeasured > 0 {
		return fmt.Errorf("%w: %d package(s) not measured, too many mutants timed out", errCannotRun, unmeasured)
	}

	_, _ = fmt.Fprintf(out, "%d changed package(s) at or over their mutation floor\n", len(floored))

	return nil
}

// reportPath is where gremlins writes a package's report, from the checkout.
func reportPath(pkg string) string {
	return filepath.ToSlash(filepath.Join(".cache", "harness", "mutation", strings.ReplaceAll(pkg, "/", "_")+".json"))
}

// gremlins mutates one package and reads its report.
func (e env) gremlins(ctx context.Context, config mutation.Config, pkg string) (mutation.Report, error) {
	report := reportPath(pkg)

	err := os.MkdirAll(filepath.Join(e.root, filepath.Dir(report)), dirPerm) // #nosec G703 -- .cache/harness/mutation of this checkout
	if err != nil {
		return mutation.Report{}, fmt.Errorf("mutation reports: %w", err)
	}

	// -short: gremlins runs each package's tests, and the integration tests need a TheHive the
	// container does not have.
	cmd := exec.CommandContext(ctx, "make", "go", "GOFLAGS=-short", "CMD="+strings.Join(config.Args(pkg, report), " ")) // #nosec G204 G702 -- the Makefile's go target; the tool comes from the reviewed config, the package from this checkout's changed paths
	cmd.Dir = e.root

	var output bytes.Buffer

	cmd.Stdout, cmd.Stderr = &output, &output

	err = cmd.Run()
	if err != nil {
		return mutation.Report{}, fmt.Errorf("gremlins on %s: %w\n%s", pkg, err, lastLines(output.String(), benchLogLines))
	}

	data, err := os.ReadFile(filepath.Join(e.root, report)) // #nosec G304 G703 -- the report gremlins just wrote under .cache/harness
	if err != nil {
		return mutation.Report{}, fmt.Errorf("read the report of %s: %w", pkg, err)
	}

	r, err := mutation.ParseReport(data)
	if err != nil {
		return mutation.Report{}, fmt.Errorf("%s: %w", pkg, err)
	}

	return r, nil
}
