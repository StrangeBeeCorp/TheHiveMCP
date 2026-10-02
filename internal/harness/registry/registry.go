// Package registry reads gates/registry.yaml: the gates the harness runs, the cadences that run
// them, and the suites of paths whose changes select them.
package registry

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Cadences lists every cadence a gate may run at, in the order the documentation shows them.
var Cadences = []string{"edit", "stop", "commit", "pr", "ci", "nightly"}

const (
	// AnySuite in a gate's suites runs the gate whatever changed.
	AnySuite = "*"
	// Everything is the suite whose selection selects every suite.
	Everything = "everything"
	// ScopePlaceholder in a gate's command is replaced by the file scope of the cadence.
	ScopePlaceholder = "{scope}"
)

// Suite is a named set of paths. A changed path selects every suite whose paths match it.
type Suite struct {
	ID      string   `yaml:"id"`
	About   string   `yaml:"about"`
	Paths   []string `yaml:"paths"`
	Implies []string `yaml:"implies"`
}

// Gate is one check with a verdict: a shell command or a check built into the harness.
type Gate struct {
	ID      string   `yaml:"id"`
	Runs    string   `yaml:"runs"`
	Builtin string   `yaml:"builtin"`
	Docker  bool     `yaml:"docker"`
	Cadence []string `yaml:"cadence"`
	Suites  []string `yaml:"suites"`
	BudgetS int      `yaml:"budget_s"`
	Blocks  string   `yaml:"blocks"`
	// Static gates read files without building them: they are the only ones the Tester's stop
	// runs, since its tests fail on purpose and may not compile yet.
	Static bool `yaml:"static"`
}

// Registry is the parsed gates/registry.yaml.
type Registry struct {
	Scopes map[string]string `yaml:"scopes"`
	Suites []Suite           `yaml:"suites"`
	Gates  []Gate            `yaml:"gates"`
}

// Load reads and validates the registry at path. Unknown keys are errors, so a typo in a field
// name cannot silently disable a gate.
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the path is the repository's own registry
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	return Parse(data, path)
}

// Parse decodes and validates a registry; path names it in errors.
func Parse(data []byte, path string) (*Registry, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var reg Registry

	err := dec.Decode(&reg)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	err = TrailingComments(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	err = reg.Validate()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return &reg, nil
}

// Validate reports every inconsistency at once: duplicate or unknown ids, unknown cadences, a
// gate that is both or neither a command and a builtin, a missing budget or blocking rule.
func (r *Registry) Validate() error {
	suiteIDs, problems := r.suiteProblems()

	for _, c := range Cadences {
		if _, ok := r.Scopes[c]; !ok {
			problems = append(problems, fmt.Sprintf("no scope for cadence %q", c))
		}
	}

	gateIDs := map[string]bool{}

	for _, g := range r.Gates {
		if gateIDs[g.ID] {
			problems = append(problems, fmt.Sprintf("gate %q is defined twice", g.ID))
		}

		gateIDs[g.ID] = true
		problems = append(problems, g.problems(suiteIDs)...)
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}

	return nil
}

// Gate returns the gate with this id.
func (r *Registry) Gate(id string) (Gate, bool) {
	for _, g := range r.Gates {
		if g.ID == id {
			return g, true
		}
	}

	return Gate{}, false
}

// Command is the shell command the gate runs at this cadence, with its scope filled in.
func (r *Registry) Command(g Gate, cadence string) string {
	return strings.ReplaceAll(g.Runs, ScopePlaceholder, r.Scopes[cadence])
}

// suiteProblems returns the suite ids and what is wrong with the suites.
func (r *Registry) suiteProblems() (map[string]bool, []string) {
	var problems []string

	ids := map[string]bool{}

	for _, s := range r.Suites {
		if ids[s.ID] {
			problems = append(problems, fmt.Sprintf("suite %q is defined twice", s.ID))
		}

		ids[s.ID] = true

		if len(s.Paths) == 0 {
			problems = append(problems, fmt.Sprintf("suite %q has no paths", s.ID))
		}
	}

	if !ids[Everything] {
		problems = append(problems, fmt.Sprintf("the %q suite is missing", Everything))
	}

	for _, s := range r.Suites {
		for _, imp := range s.Implies {
			if !ids[imp] {
				problems = append(problems, fmt.Sprintf("suite %q implies unknown suite %q", s.ID, imp))
			}
		}
	}

	return ids, problems
}

// RunsAt reports whether the gate runs at this cadence.
func (g Gate) RunsAt(cadence string) bool {
	return slices.Contains(g.Cadence, cadence)
}

func (g Gate) problems(suiteIDs map[string]bool) []string {
	var p []string

	if g.ID == "" {
		p = append(p, "a gate has no id")
	}

	if (g.Runs == "") == (g.Builtin == "") {
		p = append(p, fmt.Sprintf("gate %q needs exactly one of runs and builtin", g.ID))
	}

	if g.BudgetS <= 0 {
		p = append(p, fmt.Sprintf("gate %q has no budget_s", g.ID))
	}

	if strings.TrimSpace(g.Blocks) == "" {
		p = append(p, fmt.Sprintf("gate %q does not say what blocks", g.ID))
	}

	if len(g.Cadence) == 0 {
		p = append(p, fmt.Sprintf("gate %q has no cadence", g.ID))
	}

	for _, c := range g.Cadence {
		if !slices.Contains(Cadences, c) {
			p = append(p, fmt.Sprintf("gate %q has unknown cadence %q", g.ID, c))
		}
	}

	if len(g.Suites) == 0 {
		p = append(p, fmt.Sprintf("gate %q has no suites", g.ID))
	}

	for _, s := range g.Suites {
		if s != AnySuite && !suiteIDs[s] {
			p = append(p, fmt.Sprintf("gate %q names unknown suite %q", g.ID, s))
		}
	}

	return p
}
