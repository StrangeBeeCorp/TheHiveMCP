package registry

import (
	"slices"
	"sort"
)

// Selection is the set of suites a change selects, and why.
type Selection struct {
	// All is true when every suite is selected: no base to compare with, a change to the
	// everything suite, or a path no suite claims.
	All bool
	// Reason says why All is true.
	Reason string
	// Suites maps each selected suite to the changed paths that selected it (empty for a suite
	// selected through another's implies).
	Suites map[string][]string
	// Unclaimed lists the changed paths no suite claims.
	Unclaimed []string
	// Paths are the changed paths, when the selection came from them.
	Paths []string
	// Base is the revision the paths changed from ("" when every suite is selected).
	Base string
}

// SelectAll selects every suite, for the reason given.
func SelectAll(reason string) Selection {
	return Selection{All: true, Reason: reason, Suites: map[string][]string{}}
}

// Select maps changed paths to suites. A path no suite claims, or a path of the everything
// suite, selects every suite: the harness cannot tell what such a change affects.
func (r *Registry) Select(paths []string) Selection {
	sel := Selection{Suites: map[string][]string{}, Paths: paths}

	for _, p := range paths {
		claimed := false

		for _, s := range r.Suites {
			if MatchAny(s.Paths, p) {
				claimed = true

				sel.Suites[s.ID] = append(sel.Suites[s.ID], p)
			}
		}

		if !claimed {
			sel.Unclaimed = append(sel.Unclaimed, p)
		}
	}

	switch {
	case len(sel.Suites[Everything]) > 0:
		sel.All, sel.Reason = true, "a change to the harness or the repository-wide tooling: "+sel.Suites[Everything][0]
	case len(sel.Unclaimed) > 0:
		sel.All, sel.Reason = true, "a path no suite claims: "+sel.Unclaimed[0]
	}

	r.addImplied(sel.Suites)

	return sel
}

func (r *Registry) addImplied(selected map[string][]string) {
	queue := make([]string, 0, len(selected))
	for id := range selected {
		queue = append(queue, id)
	}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		for _, s := range r.Suites {
			if s.ID != id {
				continue
			}

			for _, imp := range s.Implies {
				if _, ok := selected[imp]; !ok {
					selected[imp] = nil
					queue = append(queue, imp)
				}
			}
		}
	}
}

// Has reports whether the suite is selected.
func (s Selection) Has(suite string) bool {
	if s.All {
		return true
	}

	_, ok := s.Suites[suite]

	return ok
}

// Names lists the selected suites in a stable order ("all" when every suite is selected).
func (s Selection) Names() []string {
	if s.All {
		return []string{"all"}
	}

	names := make([]string, 0, len(s.Suites))
	for id := range s.Suites {
		names = append(names, id)
	}

	sort.Strings(names)

	return names
}

// Selects reports whether the selection runs the gate: one of its suites is selected.
func (s Selection) Selects(g Gate) bool {
	if slices.Contains(g.Suites, AnySuite) {
		return true
	}

	return slices.ContainsFunc(g.Suites, s.Has)
}

// At lists the gates of a cadence, in registry order.
func (r *Registry) At(cadence string) []Gate {
	var gates []Gate

	for _, g := range r.Gates {
		if g.RunsAt(cadence) {
			gates = append(gates, g)
		}
	}

	return gates
}
