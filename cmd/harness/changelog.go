package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// changelog is the changelog gate: when what the model reads changed since the base, CHANGELOG.md
// gained an entry under ## [Unreleased]. A run with no base to compare with (--all, a push to
// main, no origin/main) has no change to judge: its gate is SKIPPED, not passed.
func (e env) changelog(ctx context.Context) ([]checks.Finding, error) {
	if e.base == "" {
		return nil, fmt.Errorf("%w: no base to compare with, so no change of what the model reads to judge", runner.ErrSkipped)
	}

	changed := e.changed
	if slices.Contains(changed, checks.ChangelogPath) {
		before, _ := gitx.At(ctx, e.root, e.base, checks.ChangelogPath)
		after, _ := os.ReadFile(filepath.Join(e.root, checks.ChangelogPath)) // #nosec G304 -- this checkout's changelog

		if !checks.UnreleasedAdded(string(before), string(after)) {
			changed = slices.DeleteFunc(slices.Clone(changed), func(p string) bool { return p == checks.ChangelogPath })
		}
	}

	return checks.Changelog(changed, gitx.HasPath(ctx, e.root, e.base, checks.SurfaceSummary)), nil
}

// changeSet gives `check changelog` and `check integrity` the change they judge, as a run would:
// since the merge base of --base (default $HARNESS_BASE, then origin/main) and HEAD.
func (e env) changeSet(ctx context.Context, args []string) (env, error) {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)

	var scope scopeFlags

	fs.StringVar(&scope.base, "base", os.Getenv("HARNESS_BASE"), "compare with the merge base of this ref and HEAD (default origin/main)")

	err := fs.Parse(args)
	if err != nil {
		return e, usageError{err.Error()}
	}

	sel, err := scope.selection(ctx, e)
	if err != nil {
		return e, err
	}

	e.changed, e.base = sel.Paths, sel.Base

	return e, nil
}
