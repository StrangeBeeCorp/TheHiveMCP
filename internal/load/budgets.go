package load

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// BudgetsPath holds each run's shape and budgets.
	BudgetsPath = "gates/load.yaml"
	// BaselineDir holds what each run measured when its budgets were set.
	BaselineDir = "baselines/load"
)

// What a run runs against.
const (
	AgainstStub    = "stub"
	AgainstTheHive = "thehive"
)

// ErrBudgets means gates/load.yaml is incomplete.
var ErrBudgets = errors.New("load budgets")

// Budget is what a run may not exceed. A zero limit is not checked, except MaxErrors.
type Budget struct {
	P95ms     float64 `yaml:"p95_ms"`
	P99ms     float64 `yaml:"p99_ms"`
	MaxErrors int     `yaml:"max_errors"`
	// MinPerSec is the throughput the clients must get at least.
	MinPerSec float64 `yaml:"min_calls_per_second"`
	MaxRSSMiB float64 `yaml:"max_rss_mib"`
	// MaxGrowth is the most the server's memory may grow after the warmup, in MiB per hour.
	MaxGrowth float64 `yaml:"max_growth_mib_per_hour"`
}

// Run is one run of gates/load.yaml: what it runs against, its shape and its budget.
type Run struct {
	Config `yaml:",inline"`

	// Against is stub (cmd/load, the load and soak gates) or thehive (the integration suite).
	Against string `yaml:"against"`
	Budget  Budget `yaml:"budget"`
}

// Budgets is gates/load.yaml.
type Budgets struct {
	Runs map[string]Run `yaml:"runs"`
}

// ReadBudgets reads and checks gates/load.yaml.
func ReadBudgets(p string) (Budgets, error) {
	data, err := os.ReadFile(p) // #nosec G304 -- the repository's own budgets
	if err != nil {
		return Budgets{}, fmt.Errorf("read %s: %w", p, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var b Budgets

	err = dec.Decode(&b)
	if err != nil {
		return Budgets{}, fmt.Errorf("parse %s: %w", p, err)
	}

	for name, r := range b.Runs {
		if r.Against != AgainstStub && r.Against != AgainstTheHive {
			return Budgets{}, fmt.Errorf("%w: %s: run %q runs against %q (stub or thehive)", ErrBudgets, p, name, r.Against)
		}

		soak := r.Duration > 0
		if r.Clients < 1 || r.Sample <= 0 || (!soak && r.Calls < 1) || (soak && r.Rate <= 0) {
			return Budgets{}, fmt.Errorf("%w: %s: run %q needs clients, a sample interval, and calls or a duration with a rate", ErrBudgets, p, name)
		}
	}

	return b, nil
}

// Run is the named run, or an error that lists none.
func (b Budgets) Run(name string) (Run, error) {
	r, ok := b.Runs[name]
	if !ok {
		return Run{}, fmt.Errorf("%w: no run %q in %s", ErrBudgets, name, BudgetsPath)
	}

	return r, nil
}

// Check lists what the result exceeds of the budget, one problem per line.
func (b Budget) Check(r Result) []string {
	var problems []string

	over := func(what string, got, limit float64, unit string) {
		if limit > 0 && got > limit {
			problems = append(problems, fmt.Sprintf("%s: %s %.1f %s, over its budget of %.1f %s", r.Run, what, got, unit, limit, unit))
		}
	}

	over("p95", r.P95ms, b.P95ms, "ms")
	over("p99", r.P99ms, b.P99ms, "ms")
	over("memory", r.MaxRSS(), b.MaxRSSMiB, "MiB")
	over("memory growth", r.GrowthMiBPerHour, b.MaxGrowth, "MiB/h")

	if r.Errors > b.MaxErrors {
		problems = append(problems, fmt.Sprintf("%s: %d failed calls, %d allowed (first: %s)", r.Run, r.Errors, b.MaxErrors, r.FirstError))
	}

	if b.MinPerSec > 0 && r.PerSec < b.MinPerSec {
		problems = append(problems, fmt.Sprintf("%s: %.0f calls per second, under its budget of %.0f", r.Run, r.PerSec, b.MinPerSec))
	}

	return problems
}

// Baseline is where a run's baseline is, from the checkout's root.
func Baseline(root, name string) string {
	return filepath.Join(root, BaselineDir, name+".json")
}

// Compare says how a result stands against its baseline, in one line.
func Compare(r, base Result) string {
	ratio := func(got, was float64) string {
		if was == 0 {
			return "-"
		}

		return fmt.Sprintf("x%.2f", got/was)
	}

	return fmt.Sprintf("%s against its baseline of %s: p95 %s, p99 %s, throughput %s, memory %s",
		r.Run, base.Recorded.Format("2006-01-02"), ratio(r.P95ms, base.P95ms), ratio(r.P99ms, base.P99ms),
		ratio(r.PerSec, base.PerSec), ratio(r.MaxRSS(), base.MaxRSS()))
}
