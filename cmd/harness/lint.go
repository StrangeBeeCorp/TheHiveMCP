package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/lint"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/proc"
)

// The commands that run the pinned linters and formatters.
const (
	cmdLint = "lint"
	cmdFmt  = "fmt"
)

// toolExec runs the tools' command lines, and dockerRunning says whether they can run: the
// tests replace both.
var (
	toolExec      proc.Exec = proc.Local
	dockerRunning           = dockerUp
)

// toolRun is what a run of the linters or the formatters did.
type toolRun struct {
	opts  lint.Options
	files lint.Files
	rep   lint.Report
}

// lint runs the linters: bin/harness lint [scope] [--check | --fix], read-only by default.
func (e env) lint(ctx context.Context, args []string, out io.Writer) error {
	run, err := e.tools(ctx, args, false)
	if err != nil {
		return err
	}

	files, rep := run.files, run.rep

	_, _ = io.WriteString(out, fixedBlock("Auto-fixed", rep.Fixed[lint.Format])+fixedBlock("Auto-fixed (lint)", rep.Fixed[lint.LintFix]))

	if len(rep.Failures) == 0 {
		_, _ = fmt.Fprintf(out, "✓ lint passed (%s)\n", files.Summary())
		return nil
	}

	_, _ = io.WriteString(out, failureBlocks(rep.Failures))
	_, _ = fmt.Fprintf(out, "✗ lint failed (%s): %s\n", files.Summary(), failureNames(rep.Failures))

	return errFailed
}

// fmtCmd runs the formatters: bin/harness fmt [scope] [--check | --fix], rewriting by default.
func (e env) fmtCmd(ctx context.Context, args []string, out io.Writer) error {
	run, err := e.tools(ctx, args, true)
	if err != nil {
		return err
	}

	files, rep := run.files, run.rep

	switch formatted := rep.Fixed[lint.Format]; {
	case run.opts.Fix && len(rep.Failures) > 0:
		_, _ = io.WriteString(out, fixedBlock("Formatted", formatted)+failureBlocks(rep.Failures))
		_, _ = fmt.Fprintf(out, "✗ fmt: a formatter could not run: %s\n", failureNames(rep.Failures))

		return errFailed
	case run.opts.Fix && len(formatted) > 0:
		_, _ = io.WriteString(out, fixedBlock("Formatted", formatted))
		_, _ = fmt.Fprintf(out, "✓ formatted %d of %s\n", len(formatted), files.Summary())

		return nil
	case run.opts.Fix:
		_, _ = fmt.Fprintf(out, "✓ already formatted (%s)\n", files.Summary())
		return nil
	case len(rep.Failures) == 0:
		_, _ = fmt.Fprintf(out, "✓ fmt check passed (%s)\n", files.Summary())
		return nil
	}

	_, _ = io.WriteString(out, failureBlocks(rep.Failures))
	_, _ = fmt.Fprintf(out, "✗ fmt: files need formatting (run make fmt): %s\n", failureNames(rep.Failures))

	return errFailed
}

// tools selects the scope's files, builds the steps and runs them: the formatters' alone for
// formatOnly (bin/harness fmt), else the linters'.
func (e env) tools(ctx context.Context, args []string, formatOnly bool) (toolRun, error) {
	opts, err := lint.ParseArgs(args, formatOnly)
	if err != nil {
		return toolRun{}, usageError{err.Error()}
	}

	if !dockerRunning(ctx) {
		return toolRun{}, errors.New("docker is not running: start OrbStack and run again")
	}

	paths, err := opts.Scope.Select(ctx, e.root)
	if err != nil {
		return toolRun{}, fmt.Errorf("lint: %w", err)
	}

	repo, err := e.lintRepo()
	if err != nil {
		return toolRun{}, err
	}

	files, build := lint.Classify(paths), lint.Steps
	if formatOnly {
		files, build = files.Formattable(), lint.FormatSteps
	}

	watch := files.All()
	if opts.Fix && len(files.Go) > 0 {
		// golangci-lint fmt and run --fix rewrite the whole module, not only the scope's files.
		watch, err = e.withGoFiles(ctx, watch)
		if err != nil {
			return toolRun{}, err
		}
	}

	rep := lint.Runner{Root: e.root, Exec: toolExec, Watch: watch}.Run(ctx, build(files, repo, opts.Fix))

	return toolRun{opts: opts, files: files, rep: rep}, nil
}

// lintRepo is what the steps need of the checkout: the Go images the Makefile pins, and the
// tools' configuration files.
func (e env) lintRepo() (lint.Repo, error) {
	makefile, err := os.ReadFile(filepath.Join(e.root, "Makefile")) // #nosec G304 -- the repository's Makefile
	if err != nil {
		return lint.Repo{}, fmt.Errorf("lint: read the Makefile: %w", err)
	}

	images, err := lint.MakefileImages(makefile)
	if err != nil {
		return lint.Repo{}, fmt.Errorf("lint: %w", err)
	}

	repo := lint.Repo{Root: e.root, Images: images}

	_, err = os.Stat(filepath.Join(e.root, ".hadolint.yaml"))
	repo.HadolintConfig = err == nil

	ini, err := os.ReadFile(filepath.Join(e.root, "checkmake.ini")) // #nosec G304 -- the repository's checkmake rules
	if err == nil {
		repo.CheckmakeINI = ini
	}

	return repo, nil
}

func (e env) withGoFiles(ctx context.Context, watch []string) ([]string, error) {
	all, err := gitx.Files(ctx, e.root)
	if err != nil {
		return nil, fmt.Errorf("lint: list the files: %w", err)
	}

	for _, p := range all {
		if strings.HasSuffix(p, ".go") {
			watch = append(watch, p)
		}
	}

	return watch, nil
}

func fixedBlock(label string, files []string) string {
	if len(files) == 0 {
		return ""
	}

	return label + ":\n  " + strings.Join(files, "\n  ") + "\n"
}

func failureBlocks(failures []lint.Failure) string {
	var b strings.Builder

	for _, f := range failures {
		_, _ = fmt.Fprintf(&b, "=== %s ===\n%s\n\n", f.Name, f.Output)
	}

	return b.String()
}

func failureNames(failures []lint.Failure) string {
	names := make([]string, len(failures))
	for i, f := range failures {
		names[i] = f.Name
	}

	return strings.Join(names, ", ")
}
