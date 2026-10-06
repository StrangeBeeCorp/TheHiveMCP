// Package evalsmoke is the nightly eval smoke: TheHiveMCPEval's suite run against this checkout on
// a cheap model, its cost held to a cap, and its pass rate to the previous night's. Some
// regressions come from no pull request: a provider changes a model, TheHive ships a new image.
// gates/eval.yaml holds the suite, the model, the number of cases and the cap.
package evalsmoke

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// Path holds the smoke's suite, model and cap.
	Path = "gates/eval.yaml"
	// percent turns a rate into points.
	percent = 100
	// resultPerm is a result file's mode.
	resultPerm = 0o600
)

// ErrConfig means gates/eval.yaml is incomplete.
var ErrConfig = errors.New("eval config")

// Config is gates/eval.yaml.
type Config struct {
	// Suite is the TheHiveMCPEval suite, and Model a label of its configs/providers.yaml.
	Suite string `yaml:"suite"`
	Model string `yaml:"model"`
	// Cases is how many cases the suite has: a run with fewer measured only part of it.
	Cases int `yaml:"cases"`
	// CostPerCase is the measured estimate a run is checked against before it starts.
	CostPerCase float64 `yaml:"cost_per_case_usd"`
	// Cap is the most a run may cost, in USD.
	Cap float64 `yaml:"cap_usd"`
	// MaxDrop is how far the pass rate may fall under the previous night's, from 0 to 1.
	MaxDrop float64 `yaml:"max_drop"`
}

// Load reads and checks gates/eval.yaml.
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

	if c.Suite == "" || c.Model == "" || c.Cases < 1 || c.CostPerCase <= 0 || c.Cap <= 0 || c.MaxDrop < 0 || c.MaxDrop >= 1 {
		return Config{}, fmt.Errorf("%w: %s needs a suite, a model, the cases, a cost per case, a cap and a max drop under 1", ErrConfig, p)
	}

	return c, nil
}

// Estimate is what a run should cost, before it starts.
func (c Config) Estimate() float64 { return float64(c.Cases) * c.CostPerCase }

// ErrOverCap means a run would cost, or cost, more than its cap.
var ErrOverCap = errors.New("over the eval cap")

// Preflight refuses a run whose estimate is over the cap: it is not started.
func (c Config) Preflight() error {
	if c.Estimate() > c.Cap {
		return fmt.Errorf("%w: %d cases at $%.4f is $%.3f, over $%.2f (gates/eval.yaml)", ErrOverCap, c.Cases, c.CostPerCase, c.Estimate(), c.Cap)
	}

	return nil
}

// Result is one night's smoke: the sum of the evaluations its run created.
type Result struct {
	Suite    string   `json:"suite"`
	Model    string   `json:"model"`
	Head     string   `json:"head"`
	Evals    []string `json:"evals,omitempty"`
	Cases    int      `json:"cases"`
	Passed   int      `json:"passed"`
	PassRate float64  `json:"pass_rate"`
	CostUSD  float64  `json:"cost_usd"`
	// Unanswered counts the cases that got no answer at all: they measured nothing.
	Unanswered int `json:"unanswered,omitempty"`
	// CostUnmeasured means eval-cli could not attribute the true cost of an evaluation: the cap was
	// not checked.
	CostUnmeasured bool `json:"cost_unmeasured,omitempty"`
	// OK says whether the night passed its checks: only such a night is the next one's baseline.
	OK       bool      `json:"ok"`
	Recorded time.Time `json:"recorded"`
}

// Add is the sum of two evaluations of the same night.
func (r Result) Add(o Result) Result {
	r.Cases += o.Cases
	r.Passed += o.Passed
	r.Unanswered += o.Unanswered
	r.CostUSD += o.CostUSD
	r.CostUnmeasured = r.CostUnmeasured || o.CostUnmeasured

	if r.Cases > 0 {
		r.PassRate = float64(r.Passed) / float64(r.Cases)
	}

	return r
}

// Run statuses `eval-cli eval run --summary-json` writes when the run completed: every case
// passed, or some failed. Any other status (error, interrupted) is a run that did not complete.
const (
	StatusPassed      = "passed"
	StatusTestsFailed = "tests_failed"
)

var (
	// ErrRunBroke means the run did not complete: its summary names the step that failed.
	ErrRunBroke = errors.New("the eval run did not complete")
	// ErrNoEval means the run created no evaluation for the model: it measured nothing.
	ErrNoEval = errors.New("the run created no evaluation")
)

// Run is what `eval-cli eval run --summary-json PATH` writes, whatever the run's outcome.
type Run struct {
	Model      string `json:"model"`
	ExitCode   int    `json:"exit_code"`
	Status     string `json:"status"`
	FailedStep string `json:"failed_step"`
	// EvalIDs are the evaluations the run created, one per phase of the suite (read, then write).
	EvalIDs     []string     `json:"eval_ids"`
	Evaluations []Evaluation `json:"evaluations"`
}

// Evaluation is one phase's count. Errors counts the rows that got no answer at all.
type Evaluation struct {
	EvalID string `json:"eval_id"`
	Total  int    `json:"total"`
	Passed int    `json:"passed"`
	Errors int    `json:"errors"`
}

// ReadRun reads a run's summary.
func ReadRun(data []byte) (Run, error) {
	var r Run

	err := json.Unmarshal(data, &r)
	if err != nil {
		return Run{}, fmt.Errorf("read eval-cli's run summary: %w", err)
	}

	return r, nil
}

// Result is the night a completed run measured, the sum of its evaluations. A run that did not
// complete, or that created no evaluation for the model, measured nothing.
func (r Run) Result(model string) (Result, error) {
	if r.Status != StatusPassed && r.Status != StatusTestsFailed {
		return Result{}, fmt.Errorf("%w: status %q, exit %d, at the step %q", ErrRunBroke, r.Status, r.ExitCode, r.FailedStep)
	}

	if r.Model != model {
		return Result{}, fmt.Errorf("%w on %q: the run was on %q", ErrNoEval, model, r.Model)
	}

	night := Result{Model: model, Evals: r.EvalIDs}
	for _, e := range r.Evaluations {
		night = night.Add(Result{Cases: e.Total, Passed: e.Passed, Unanswered: e.Errors})
	}

	if night.Cases == 0 {
		return Result{}, fmt.Errorf("%w on %q", ErrNoEval, model)
	}

	return night, nil
}

// TrueCost is the model's cost from `eval-cli eval cost`, which reconciles LiteLLM's spend log, or
// false when the report has none for the model.
func TrueCost(data []byte, model string) (float64, bool) {
	var report struct {
		Columns map[string]struct {
			CostUSD *float64 `json:"cost_usd"`
		} `json:"columns"`
	}

	if json.Unmarshal(data, &report) != nil {
		return 0, false
	}

	c, ok := report.Columns[model]
	if !ok || c.CostUSD == nil {
		return 0, false
	}

	return *c.CostUSD, true
}

// Check lists what is wrong with a night's result: part of the suite measured, cases with no
// answer, a cost over the cap, or a pass rate that fell more than MaxDrop under the previous night's (nil: no previous).
func (c Config) Check(r Result, previous *Result) []string {
	var problems []string

	switch {
	case r.Cases < c.Cases:
		problems = append(problems, fmt.Sprintf("only %d of the %d cases of %s ran", r.Cases, c.Cases, c.Suite))
	case r.Cases > c.Cases:
		problems = append(problems, fmt.Sprintf("%d cases read for the %d of %s: evaluations that are not this run's were read", r.Cases, c.Cases, c.Suite))
	}

	if r.Unanswered > 0 {
		problems = append(problems, fmt.Sprintf("%d case(s) got no answer and measured nothing: rerun them before trusting the pass rate", r.Unanswered))
	}

	if r.CostUnmeasured {
		problems = append(problems, "eval-cli could not attribute the run's true cost (eval-cli eval cost): the cap was not checked")
	}

	if r.CostUSD > c.Cap {
		problems = append(problems, fmt.Sprintf("the run cost $%.3f, over its cap of $%.2f", r.CostUSD, c.Cap))
	}

	if previous != nil && previous.Model == r.Model && r.PassRate < previous.PassRate-c.MaxDrop {
		problems = append(problems, fmt.Sprintf("%s's pass rate fell from %.0f%% (%s) to %.0f%%, more than %.0f points",
			r.Model, percent*previous.PassRate, previous.Recorded.Format(time.DateOnly), percent*r.PassRate, percent*c.MaxDrop))
	}

	return problems
}

// Summary is the result in one line, next to the previous night's when there is one.
func Summary(r Result, previous *Result) string {
	s := fmt.Sprintf("%s on %s: %d/%d passed (%.0f%%), $%.3f", r.Suite, r.Model, r.Passed, r.Cases, percent*r.PassRate, r.CostUSD)
	if previous != nil {
		s += fmt.Sprintf("; previous night %d/%d (%.0f%%, %s)", previous.Passed, previous.Cases, percent*previous.PassRate, previous.Recorded.Format(time.DateOnly))
	}

	return s
}

// Write saves a result as JSON.
func (r Result) Write(p string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the result: %w", err)
	}

	err = os.WriteFile(p, append(data, '\n'), resultPerm)
	if err != nil {
		return fmt.Errorf("write %s: %w", p, err)
	}

	return nil
}

// Read reads a result Write saved.
func Read(p string) (Result, error) {
	data, err := os.ReadFile(p) // #nosec G304 -- a result a night's job saved
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", p, err)
	}

	var r Result

	err = json.Unmarshal(data, &r)
	if err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", p, err)
	}

	return r, nil
}
