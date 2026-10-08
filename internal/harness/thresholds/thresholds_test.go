package thresholds

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

// The test thresholds' ids.
const (
	idAge     = "age"
	idCov     = "cov"
	idRelease = "release"
)

// The repository's own thresholds load; a file with an unknown key or metric, a duplicate id, or
// a threshold with no limit or two is refused.
func TestTheThresholdsAreLoadedAndHeldToTheirShape(t *testing.T) {
	t.Parallel()

	c, err := Load(filepath.Join("..", "..", "..", Path))
	require.NoError(t, err)
	assert.Len(t, c.Thresholds, 4)

	for name, body := range map[string]string{
		"unknown key":    "thresholds:\n  - {id: a, metric: coverage, max: 1, typo: 2}\n",
		"unknown metric": "thresholds:\n  - {id: a, metric: speed, max: 1}\n",
		"duplicate id":   "thresholds:\n  - {id: a, metric: coverage, max: 1}\n  - {id: a, metric: ci_minutes, max: 1}\n",
		"no limit":       "thresholds:\n  - {id: a, metric: coverage}\n",
		"two limits":     "thresholds:\n  - {id: a, metric: coverage, max: 1, max_drop: 2}\n",
		"no id":          "thresholds:\n  - {metric: coverage, max: 1}\n",
	} {
		p := filepath.Join(t.TempDir(), "metrics.yaml")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))

		_, err := Load(p)
		require.Error(t, err, name)
	}

	_, err = Load(filepath.Join(t.TempDir(), "absent.yaml"))
	require.Error(t, err)
}

// Each kind of limit, crossed and not: a maximum, a drop since the previous night, and a rise over
// the metric's own baseline. Without a previous night or a baseline there is nothing to cross; a
// metric not measured cannot be held to anything.
func TestEachKindOfLimitIsHeld(t *testing.T) {
	t.Parallel()

	c := Config{Thresholds: []Threshold{
		{ID: idAge, Metric: OldestFindingDays, Max: new(14.0)},
		{ID: idCov, Metric: Coverage, MaxDrop: new(2.0)},
		{ID: "ci", Metric: CIMinutes, MaxRisePercent: new(25.0)},
		{ID: idRelease, Metric: DaysSinceRelease, Max: new(60.0)},
	}}

	verdicts := func(tonight, previous map[string]Value) map[string]runner.Verdict {
		out := map[string]runner.Verdict{}
		for _, o := range Evaluate(c, tonight, previous) {
			out[o.Gate] = o.Verdict
		}

		return out
	}

	crossed := map[string]Value{OldestFindingDays: {Now: 15}, Coverage: {Now: 61.9}, CIMinutes: {Now: 12.6, Baseline: 10}}
	assert.Equal(t, map[string]runner.Verdict{idAge: runner.Fail, idCov: runner.Fail, "ci": runner.Fail, idRelease: runner.Unavailable},
		verdicts(crossed, map[string]Value{Coverage: {Now: 64}}))

	under := map[string]Value{OldestFindingDays: {Now: 14}, Coverage: {Now: 62}, CIMinutes: {Now: 12.5, Baseline: 10}, DaysSinceRelease: {Now: 2}}
	assert.Equal(t, map[string]runner.Verdict{idAge: runner.Pass, idCov: runner.Pass, "ci": runner.Pass, idRelease: runner.Pass},
		verdicts(under, map[string]Value{Coverage: {Now: 64}}), "at the limit is not past it")

	first := map[string]Value{Coverage: {Now: 10}, CIMinutes: {Now: 99, Baseline: math.NaN()}, OldestFindingDays: {Now: math.NaN()}}
	got := verdicts(first, nil)
	assert.Equal(t, runner.Pass, got[idCov], "no previous night")
	assert.Equal(t, runner.Pass, got["ci"], "no baseline")
	assert.Equal(t, runner.Unavailable, got[idAge], "not measured")

	o := Evaluate(c, crossed, map[string]Value{Coverage: {Now: 64}})[1]
	assert.Equal(t, "coverage is 61.9, 64.0 the night before: a drop of 2.1, for at most 2.0", o.Summary)
}
