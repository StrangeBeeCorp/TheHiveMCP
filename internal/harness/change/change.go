package change

import (
	"context"
	"fmt"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
)

// Metrics is what a pull request's own change does, from the merge base of its base (From) to
// its commit (Head).
type Metrics struct {
	Against    string
	From, Head string
	// Coverage is nil when make pr kept no coverage profile for the commit.
	Coverage *Coverage
	Moves    []Move
	Markers  Markers
	Deps     []string
}

const goMod = "go.mod"

// Measure measures the change of HEAD since its merge base with against. profile is the unit
// suite's coverage profile at HEAD, or nil.
func Measure(ctx context.Context, root, against string, profile []byte, lim checks.StructureLimits) (Metrics, error) {
	patch, err := gitx.Diff(ctx, root, against)
	if err != nil {
		return Metrics{}, fmt.Errorf("change metrics: %w", err)
	}

	m := Metrics{Against: against, From: gitx.MergeBase(ctx, root, against), Head: gitx.Head(ctx, root)}
	at := func(rev string) func(string) ([]byte, bool) {
		return func(p string) ([]byte, bool) { return gitx.At(ctx, root, rev, p) }
	}
	onBase, onBranch := at(m.From), at(m.Head)
	d := ParseDiff(patch)

	if profile != nil {
		gomod, _ := onBranch(goMod)

		blocks, err := ParseProfile(profile, module(gomod))
		if err != nil {
			return Metrics{}, fmt.Errorf("change metrics: %w", err)
		}

		c := DiffCoverage(d, blocks)
		m.Coverage = &c
	}

	m.Moves = Moves(d, lim, onBase, onBranch)
	m.Markers = Mark(d, func(f string) bool {
		content, ok := onBranch(f)
		_, _, code := lim.LineLimit(f, content)

		return ok && code
	})

	was, _ := onBase(goMod)
	now, _ := onBranch(goMod)
	m.Deps = NewDependencies(was, now)

	return m, nil
}

// module is the module path a go.mod declares.
func module(gomod []byte) string {
	for line := range strings.SplitSeq(string(gomod), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(name), `"`)
		}
	}

	return ""
}
