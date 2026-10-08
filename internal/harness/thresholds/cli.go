package thresholds

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/dashboard"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/issues"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

// ErrUsage is a command line the thresholds cannot run.
var ErrUsage = errors.New("usage: harness metrics [--runs DIR] [--previous FILE] [--out FILE] --run-url URL [--dry-run]")

const (
	// outPath is where tonight's values are written, for the next night to compare with.
	outPath = ".cache/harness/metrics/metrics.json"
	// weeksRead is how far back the metrics need the data: CI's last week and the four before.
	weeksRead = 6
)

// Issues is what the command does with the metrics issues: issues.GH, or a test's fake.
type Issues interface {
	List(label string) ([]triage.Issue, error)
	Label(name, color, description string) error
	ApplyAs(a triage.Action, label string) error
}

// Command measures the metrics, holds them to gates/metrics.yaml, writes tonight's values, and
// files, comments on and closes the metrics issues (bin/harness metrics, in the nightly's triage
// job). --dry-run prints what would change.
func Command(ctx context.Context, root string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("metrics", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	runs := fs.String("runs", "", "the night's artifacts: tonight's coverage is read from the coverage log among them")
	previous := fs.String("previous", "", "the last night's values (its metrics.json), for a max_drop")
	dest := fs.String("out", outPath, "where to write tonight's values, from the repository's root")
	runURL := fs.String("run-url", "", "the nightly run's URL, quoted in each issue")
	dry := fs.Bool("dry-run", false, "print what would change, change nothing")

	err := fs.Parse(args)
	if err != nil || *runURL == "" || fs.NArg() > 0 {
		return ErrUsage
	}

	d := dashboard.Collect(ctx, dashboard.OptionsFor(ctx, root, time.Now(), weeksRead))

	return Run(Night{
		Root: root, Data: d, Runs: *runs, Previous: *previous, Out: filepath.Join(root, *dest), RunURL: *runURL, Dry: *dry,
		Issues: issues.GH{Ctx: ctx, Dir: root},
	}, out)
}

// Night is one night's run of the thresholds.
type Night struct {
	Root     string
	Data     dashboard.Data
	Runs     string
	Previous string
	Out      string
	RunURL   string
	Dry      bool
	Issues   Issues
}

// Run holds the night's values to the thresholds and changes the issues to match.
func Run(n Night, out io.Writer) error {
	c, err := Load(filepath.Join(n.Root, Path))
	if err != nil {
		return fmt.Errorf("metrics: %w", err)
	}

	tonight := Values(n.Data, n.Runs)

	last, err := readValues(n.Previous)
	if err != nil {
		return fmt.Errorf("metrics: %w", err)
	}

	open, err := n.Issues.List(Label)
	if err != nil {
		return fmt.Errorf("metrics: %w", err)
	}

	recall(c, last, open)

	outcomes := Evaluate(c, tonight, last)
	for _, o := range outcomes {
		_, _ = fmt.Fprintf(out, "%-8s %s: %s\n", o.Verdict, o.Gate, o.Summary)
	}

	next := Carry(c, tonight, last, outcomes)

	err = n.change(Plan(c, outcomes, open, n.RunURL, levels(c, next)), out)
	if err != nil {
		return err
	}

	// Written last: a night whose issues could not be changed leaves no values, and the next night
	// compares with the last one that did, so a crossing it could not file is filed then.
	err = writeValues(n.Out, next)
	if err != nil {
		return fmt.Errorf("metrics: %w", err)
	}

	return nil
}

// recall gives a drop the level its open issue is measured from, on a night without the last
// night's value of it (its artifact expired, or could not be downloaded): the issue then still
// closes only once the metric is back within its limit of that level.
func recall(c Config, last map[string]Value, open []triage.Issue) {
	for _, t := range c.Thresholds {
		if v, ok := last[t.Metric]; t.MaxDrop == nil || (ok && !math.IsNaN(v.Now)) {
			continue
		}

		for _, i := range open {
			if level, ok := levelOf(i); ok && thresholdOf(i) == t.ID {
				last[t.Metric] = Value{Now: level, Reference: level}
			}
		}
	}
}

// levels are, for each drop that stays crossed, the level it is measured from, by threshold.
func levels(c Config, next map[string]Value) map[string]float64 {
	out := map[string]float64{}

	for _, t := range c.Thresholds {
		if v, ok := next[t.Metric]; ok && t.MaxDrop != nil && v.Reference > 0 {
			out[t.ID] = v.Reference
		}
	}

	return out
}

// change files, comments on and closes the metrics issues; a dry run prints it.
func (n Night) change(actions []triage.Action, out io.Writer) error {
	var err error

	for _, a := range actions {
		_, _ = fmt.Fprintf(out, "%s %s\n", a.Kind, cmpOr(a.Title, fmt.Sprintf("#%d", a.Issue)))
	}

	if n.Dry {
		return nil
	}

	if slices.ContainsFunc(actions, func(a triage.Action) bool { return a.Kind == triage.Open }) {
		err = n.Issues.Label(Label, LabelColor, LabelDescription)
		if err != nil {
			return fmt.Errorf("metrics: %w", err)
		}
	}

	for _, a := range actions {
		err = n.Issues.ApplyAs(a, Label)
		if err != nil {
			return fmt.Errorf("metrics: %w", err)
		}
	}

	return nil
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}

	return s
}

// Values are tonight's metrics: the coverage of the night's coverage log (this clone's last make
// pr without one), and the dashboard's measures. A measure whose source was not read is unknown,
// not a zero: no issue open is not the same as no issue read.
func Values(d dashboard.Data, runs string) map[string]Value {
	m := dashboard.Measure(d)

	coverage := math.NaN()
	if total, ok := nightCoverage(runs); ok {
		coverage = total
	} else if runs == "" && d.Local.Coverage >= 0 {
		coverage = d.Local.Coverage
	}

	oldest := m.OldestFindingDays
	if _, ok := d.Missing[dashboard.SourceIssues]; ok {
		oldest = math.NaN()
	}

	if _, ok := d.Missing[dashboard.SourceGitHub]; ok {
		oldest = math.NaN()
	}

	return map[string]Value{
		Coverage:          {Now: coverage, Baseline: math.NaN()},
		CIMinutes:         {Now: m.CIRecent, Baseline: m.CIBefore},
		OldestFindingDays: {Now: oldest, Baseline: math.NaN()},
		DaysSinceRelease:  {Now: m.DaysSinceRelease, Baseline: math.NaN()},
	}
}

// nightCoverage is the total of the coverage log the night's artifacts hold.
func nightCoverage(runs string) (float64, bool) {
	if runs == "" {
		return 0, false
	}

	var logs []string

	_ = filepath.WalkDir(runs, func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() && e.Name() == "coverage.log" {
			logs = append(logs, p)
		}

		return nil
	})

	slices.Sort(logs)

	for _, log := range slices.Backward(logs) {
		if total, ok := dashboard.CoverageOf(log); ok {
			return total, true
		}
	}

	return 0, false
}

// readValues reads a night's values; none when there is no file (the first night).
func readValues(p string) (map[string]Value, error) {
	if p == "" {
		return map[string]Value{}, nil
	}

	data, err := os.ReadFile(p) // #nosec G304 -- the previous night's artifact, which the workflow downloaded
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Value{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read the previous night's values: %w", err)
	}

	var v map[string]Value

	err = json.Unmarshal(data, &v)
	if err != nil {
		return nil, fmt.Errorf("read the previous night's values: %w", err)
	}

	return v, nil
}

// writeValues writes the known values: JSON has no NaN, and an unknown value is left out.
func writeValues(p string, values map[string]Value) error {
	known := map[string]Value{}

	for k, v := range values {
		if math.IsNaN(v.Now) {
			continue
		}

		if math.IsNaN(v.Baseline) {
			v.Baseline = 0
		}

		known[k] = v
	}

	data, err := json.MarshalIndent(known, "", "  ")
	if err != nil {
		return fmt.Errorf("write tonight's values: %w", err)
	}

	err = os.MkdirAll(filepath.Dir(p), 0o750) //nolint:mnd // the owner's and the group's
	if err != nil {
		return fmt.Errorf("write tonight's values: %w", err)
	}

	err = os.WriteFile(p, append(data, '\n'), 0o600) //nolint:mnd // the owner's
	if err != nil {
		return fmt.Errorf("write tonight's values: %w", err)
	}

	return nil
}
