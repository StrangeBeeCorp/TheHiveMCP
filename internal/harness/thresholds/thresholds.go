// Package thresholds holds the repository's metrics to gates/metrics.yaml every night: the coverage
// falling, CI's time growing, a finding left open, no release for long. A threshold crossed files
// one issue labelled metrics, kept open while it stays crossed and closed once the metric is back
// under it, as the triage does for the gates (internal/harness/triage).
package thresholds

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/triage"
)

// Path is the thresholds' file.
const Path = "gates/metrics.yaml"

// The metrics a threshold can name.
const (
	Coverage          = "coverage"            // total statement coverage of the night, in %
	CIMinutes         = "ci_minutes"          // median minutes of main's green CI runs, last week
	OldestFindingDays = "oldest_finding_days" // the oldest open nightly finding or debt issue
	DaysSinceRelease  = "days_since_release"  // since the last release tag
)

// shard is the outcomes' shard: the metrics are their own part of the night.
const shard = "metrics"

const hundred = 100.0

// Threshold is one line of gates/metrics.yaml: a metric and the one limit it is held to.
type Threshold struct {
	ID     string `yaml:"id"`
	Metric string `yaml:"metric"`
	Why    string `yaml:"why"`
	// Max is the highest value the metric may take.
	Max *float64 `yaml:"max"`
	// MaxDrop is how many points the metric may fall since the previous night.
	MaxDrop *float64 `yaml:"max_drop"`
	// MaxRisePercent is how far the metric may rise over its own baseline, in percent.
	MaxRisePercent *float64 `yaml:"max_rise_percent"`
}

// Config is gates/metrics.yaml.
type Config struct {
	Thresholds []Threshold `yaml:"thresholds"`
}

var errConfig = errors.New("invalid thresholds")

// Load reads the thresholds, rejecting an unknown key or metric, a duplicate id, and a threshold
// that names no limit or more than one.
func Load(p string) (Config, error) {
	data, err := os.ReadFile(p) // #nosec G304 -- the repository's own thresholds
	if err != nil {
		return Config{}, fmt.Errorf("read the thresholds: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var c Config

	err = dec.Decode(&c)
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", p, err)
	}

	seen := map[string]bool{}

	for _, t := range c.Thresholds {
		limits := 0

		for _, l := range []*float64{t.Max, t.MaxDrop, t.MaxRisePercent} {
			if l != nil {
				limits++
			}
		}

		switch {
		case t.ID == "" || seen[t.ID]:
			return Config{}, fmt.Errorf("%w: %s: a threshold needs an id of its own (%q)", errConfig, p, t.ID)
		case !known(t.Metric):
			return Config{}, fmt.Errorf("%w: %s: %s: unknown metric %q", errConfig, p, t.ID, t.Metric)
		case limits != 1:
			return Config{}, fmt.Errorf("%w: %s: %s: name one of max, max_drop and max_rise_percent", errConfig, p, t.ID)
		}

		seen[t.ID] = true
	}

	return c, nil
}

func known(metric string) bool {
	switch metric {
	case Coverage, CIMinutes, OldestFindingDays, DaysSinceRelease:
		return true
	}

	return false
}

// Value is a metric tonight, and the baseline its own data compares it with (CI's time over the
// weeks before); NaN for what is not known. Reference is the level a max_drop is measured from
// while its threshold stays crossed: the level before the drop, so that the issue stays open until
// the metric is back within its limit of it.
type Value struct {
	Now       float64 `json:"now"`
	Baseline  float64 `json:"baseline,omitempty"`
	Reference float64 `json:"reference,omitempty"`
}

// from is the level a drop is measured from: the reference the night carried, else its value.
func (v Value) from() float64 {
	if v.Reference > 0 {
		return v.Reference
	}

	return v.Now
}

// Evaluate holds tonight's values to the thresholds. previous holds the last night's values, for a
// max_drop. A metric not measured tonight, a drop with no night before it and a rise with no
// baseline cannot be held to anything: their finding cannot run, which comments on an open issue
// and closes none.
func Evaluate(c Config, tonight map[string]Value, previous map[string]Value) []triage.Outcome {
	out := make([]triage.Outcome, 0, len(c.Thresholds))

	for _, t := range c.Thresholds {
		v, ok := tonight[t.Metric]
		o := triage.Outcome{Shard: shard, Gate: t.ID}

		switch {
		case !ok || math.IsNaN(v.Now):
			o.Verdict, o.Summary = runner.Unavailable, t.Metric+" could not be measured tonight"
		case t.Max != nil:
			o.Verdict, o.Summary = verdict(v.Now > *t.Max), fmt.Sprintf("%s is %.1f, for a maximum of %.0f", t.Metric, v.Now, *t.Max)
		case t.MaxDrop != nil:
			o.Verdict, o.Summary = drop(t, v, previous)
		default:
			o.Verdict, o.Summary = rise(t, v)
		}

		out = append(out, o)
	}

	return out
}

func drop(t Threshold, v Value, previous map[string]Value) (runner.Verdict, string) {
	last, ok := previous[t.Metric]
	if !ok || math.IsNaN(last.Now) {
		return runner.Unavailable, fmt.Sprintf("%s is %.1f; no previous night to compare it with", t.Metric, v.Now)
	}

	from := last.from()
	fell := from - v.Now

	return verdict(fell > *t.MaxDrop), fmt.Sprintf("%s is %.1f, from %.1f: a drop of %.1f, for at most %.1f", t.Metric, v.Now, from, fell, *t.MaxDrop)
}

func rise(t Threshold, v Value) (runner.Verdict, string) {
	if math.IsNaN(v.Baseline) || v.Baseline <= 0 {
		return runner.Unavailable, fmt.Sprintf("%s is %.1f; no baseline to compare it with", t.Metric, v.Now)
	}

	rose := (v.Now - v.Baseline) / v.Baseline * hundred

	return verdict(rose > *t.MaxRisePercent), fmt.Sprintf("%s is %.1f, against %.1f before: %+.0f %%, for at most +%.0f %%", t.Metric, v.Now, v.Baseline, rose, *t.MaxRisePercent)
}

// Carry is what tonight leaves for the next night to compare with: tonight's values, the last
// night's for a metric not measured tonight (one blind night must not erase the level a drop is
// measured from), and, for a max_drop still crossed, the level before the drop.
func Carry(c Config, tonight, previous map[string]Value, outcomes []triage.Outcome) map[string]Value {
	next := map[string]Value{}

	for k, v := range tonight {
		if !math.IsNaN(v.Now) {
			next[k] = v
		} else if last, ok := previous[k]; ok {
			next[k] = last
		}
	}

	crossed := map[string]bool{}

	for _, o := range outcomes {
		crossed[o.Gate] = o.Verdict == runner.Fail
	}

	for _, t := range c.Thresholds {
		v, measured := tonight[t.Metric]
		if t.MaxDrop == nil || !crossed[t.ID] || !measured || math.IsNaN(v.Now) {
			continue
		}

		v.Reference = previous[t.Metric].from()
		next[t.Metric] = v
	}

	return next
}

func verdict(crossed bool) runner.Verdict {
	if crossed {
		return runner.Fail
	}

	return runner.Pass
}
