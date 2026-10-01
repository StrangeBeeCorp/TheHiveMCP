// Package mutation holds the packages a change touches to the mutation floors of
// gates/mutation.yaml: gremlins mutates their code, runs their tests, and the share of mutants the
// tests kill (the test efficacy) may not fall under the package's floor.
package mutation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is gates/mutation.yaml.
type Config struct {
	// Tool is gremlins' module at a pinned version, run with go run.
	Tool string `yaml:"tool"`
	// TimeoutCoefficient scales the time a mutant's test run may take, from the unmutated run's.
	// Unit tests take milliseconds and each mutant is rebuilt first: too low, and the mutants time
	// out instead of being killed or living, which leaves the efficacy resting on a few.
	TimeoutCoefficient int `yaml:"timeout_coefficient"`
	// Floors are each package's lowest acceptable test efficacy, in percent.
	Floors map[string]float64 `yaml:"floors"`
}

// ErrConfig means gates/mutation.yaml is incomplete.
var ErrConfig = errors.New("mutation config")

// Load reads and checks gates/mutation.yaml.
func Load(p string) (Config, error) {
	data, err := os.ReadFile(p) // #nosec G304 -- the repository's own config
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", p, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var c Config

	err = dec.Decode(&c)
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", p, err)
	}

	if !strings.Contains(c.Tool, "@v") || c.TimeoutCoefficient < 1 || len(c.Floors) == 0 {
		return Config{}, fmt.Errorf("%w: %s needs a tool pinned at a version, timeout_coefficient ≥ 1 and floors", ErrConfig, p)
	}

	return c, nil
}

// Args are the go arguments that mutate pkg and write gremlins' report to output.
func (c Config) Args(pkg, output string) []string {
	return []string{"run", c.Tool, "unleash", "--timeout-coefficient", strconv.Itoa(c.TimeoutCoefficient), "--output", output, "./" + pkg}
}

// Packages are the packages of the changed Go files that have a floor, and those that have none.
func (c Config) Packages(changed []string) (floored, unfloored []string) {
	seen := map[string]bool{}

	for _, p := range changed {
		if !strings.HasSuffix(p, ".go") || strings.Contains(p, "/testdata/") {
			continue
		}

		pkg := path.Dir(p)
		if seen[pkg] {
			continue
		}

		seen[pkg] = true

		if _, ok := c.Floors[pkg]; ok {
			floored = append(floored, pkg)
		} else {
			unfloored = append(unfloored, pkg)
		}
	}

	slices.Sort(floored)
	slices.Sort(unfloored)

	return floored, unfloored
}

// Report is what gremlins writes with --output.
type Report struct {
	Efficacy   float64 `json:"test_efficacy"`
	Coverage   float64 `json:"mutations_coverage"`
	Killed     int     `json:"mutants_killed"`
	Lived      int     `json:"mutants_lived"`
	NotCovered int     `json:"mutants_not_covered"`
	NotViable  int     `json:"mutants_not_viable"`
	// Statistics counts the mutants found, per mutator.
	Statistics map[string]int `json:"mutator_statistics"`
}

// TimedOut is how many mutants timed out: found, but neither killed, lived, uncovered nor unviable.
func (r Report) TimedOut() int {
	found := 0
	for _, n := range r.Statistics {
		found += n
	}

	return max(0, found-r.Killed-r.Lived-r.NotCovered-r.NotViable)
}

// maxTimedOut is the share of the covered mutants that may time out before the efficacy no longer
// says much.
const maxTimedOut = 0.1

// ParseReport reads gremlins' report.
func ParseReport(data []byte) (Report, error) {
	var r Report

	err := json.Unmarshal(data, &r)
	if err != nil {
		return Report{}, fmt.Errorf("parse the gremlins report: %w", err)
	}

	return r, nil
}

// Check says whether a package's report is under its floor, and the line that says how it did.
func (c Config) Check(pkg string, r Report) (string, bool) {
	floor := c.Floors[pkg]
	line := fmt.Sprintf("%s: efficacy %.1f%% (floor %.0f%%), %d killed, %d lived, %d timed out, %d not covered",
		pkg, r.Efficacy, floor, r.Killed, r.Lived, r.TimedOut(), r.NotCovered)

	if covered := r.Killed + r.Lived + r.TimedOut(); r.TimedOut() > max(2, int(maxTimedOut*float64(covered))) {
		return line + ": too many mutants timed out for the efficacy to mean much: raise timeout_coefficient in gates/mutation.yaml", false
	}

	if r.Killed+r.Lived > 0 && r.Efficacy < floor {
		return line + ": add the tests that kill the mutants that lived (gremlins' log names them)", false
	}

	return line, true
}

// Floored lists the floors in a stable order, for the summary.
func (c Config) Floored() []string {
	pkgs := make([]string, 0, len(c.Floors))
	for pkg := range c.Floors {
		pkgs = append(pkgs, pkg)
	}

	sort.Strings(pkgs)

	return pkgs
}
