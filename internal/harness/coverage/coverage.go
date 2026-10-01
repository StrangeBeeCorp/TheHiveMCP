// Package coverage reads a Go coverage profile per package and holds each package to the floor
// gates/coverage.yaml sets for it: a measurement minus a point of slack.
package coverage

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Slack is how far under its measurement a proposed floor sits, in points, so that an unrelated
// change does not trip it.
const Slack = 1.0

// percent is the scale of a coverage ratio.
const percent = 100

// Floors is gates/coverage.yaml: each package's lowest acceptable coverage, in percent of its
// statements, keyed by the package's path in the module (internal/utils).
type Floors struct {
	Floors map[string]float64 `yaml:"floors"`
}

// Package is a package's measured coverage.
type Package struct {
	Statements int
	Covered    int
}

// Percent is the share of the package's statements the tests ran.
func (p Package) Percent() float64 {
	if p.Statements == 0 {
		return 0
	}

	return percent * float64(p.Covered) / float64(p.Statements)
}

// ErrProfile means the profile is not what go test -coverprofile writes.
var ErrProfile = errors.New("not a coverage profile")

// Parse reads a profile and sums it per package, with the module's prefix removed. A block that
// appears more than once (several test binaries) counts once, covered if any run covered it.
func Parse(r io.Reader, module string) (map[string]Package, error) {
	type block struct {
		pkg   string
		stmts int
	}

	blocks := map[string]block{}
	covered := map[string]bool{}
	scanner := bufio.NewScanner(r)

	for line := 0; scanner.Scan(); line++ {
		text := scanner.Text()
		if line == 0 && strings.HasPrefix(text, "mode:") || text == "" {
			continue
		}

		// file.go:10.20,12.3 2 1: the block, its statements, how many times it ran.
		fields := strings.Fields(text)
		file, _, ok := strings.Cut(text, ":")

		const fieldCount = 3
		if !ok || len(fields) != fieldCount {
			return nil, fmt.Errorf("%w: line %d: %q", ErrProfile, line+1, text)
		}

		stmts, err1 := strconv.Atoi(fields[1])
		count, err2 := strconv.Atoi(fields[2])

		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("%w: line %d: %q", ErrProfile, line+1, text)
		}

		key := fields[0]
		blocks[key] = block{pkg: strings.TrimPrefix(path.Dir(file), module+"/"), stmts: stmts}
		covered[key] = covered[key] || count > 0
	}

	err := scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("read the profile: %w", err)
	}

	pkgs := map[string]Package{}

	for key, b := range blocks {
		p := pkgs[b.pkg]
		p.Statements += b.stmts

		if covered[key] {
			p.Covered += b.stmts
		}

		pkgs[b.pkg] = p
	}

	return pkgs, nil
}

// Check lists the packages under their floor, and the floors whose package was not measured
// (renamed, removed, or no longer tested).
func Check(measured map[string]Package, floors Floors) []string {
	var problems []string

	for pkg, floor := range floors.Floors {
		m, ok := measured[pkg]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: has a floor of %.1f%% but no coverage: remove or move its floor", pkg, floor))
		case m.Percent() < floor:
			problems = append(problems, fmt.Sprintf("%s: %.1f%% of %d statements, under its floor of %.1f%%: "+
				"test the code you added, or lower the floor with a person's yes", pkg, m.Percent(), m.Statements, floor))
		}
	}

	sort.Strings(problems)

	return problems
}

// Propose is a floor per measured package: its coverage minus Slack, rounded down to a point. A
// package whose floor would be zero (its tests are integration tests) gets none: it could not fail.
func Propose(measured map[string]Package) Floors {
	floors := Floors{Floors: map[string]float64{}}

	for pkg, m := range measured {
		if floor := math.Floor(m.Percent() - Slack); floor > 0 {
			floors.Floors[pkg] = floor
		}
	}

	return floors
}

// Summary is one line on the measured packages: how many, and how many have no floor.
func Summary(measured map[string]Package, floors Floors) string {
	missing := 0

	for pkg, m := range measured {
		if _, ok := floors.Floors[pkg]; !ok && m.Covered > 0 {
			missing++
		}
	}

	return fmt.Sprintf("%d packages measured, %d floors held, %d tested packages without a floor", len(measured), len(floors.Floors), missing)
}
