package evalsmoke

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	model = "mimo-v2.6-flash"
	suite = "mcp-tools"
)

func config() Config {
	return Config{Suite: suite, Model: model, Cases: 37, CostPerCase: 0.0015, Cap: 0.10, MaxDrop: 0.10}
}

// The repository's config loads, and its estimate is within its cap.
func TestTheRepositoryConfigLoads(t *testing.T) {
	t.Parallel()

	c, err := Load(filepath.Join("..", "..", "..", Path))
	require.NoError(t, err)
	assert.Equal(t, suite, c.Suite)
	assert.Equal(t, model, c.Model)
	require.NoError(t, c.Preflight(), "the night's estimate is within its cap")
}

// A config missing a field, or with an unknown one, is refused.
func TestAnIncompleteConfigIsRefused(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"no model":         "suite: s\ncases: 1\ncost_per_case_usd: 0.1\ncap_usd: 1\nmax_drop: 0.1\n",
		"no cap":           "suite: s\nmodel: m\ncases: 1\ncost_per_case_usd: 0.1\nmax_drop: 0.1\n",
		"a drop of 100 %":  "suite: s\nmodel: m\ncases: 1\ncost_per_case_usd: 0.1\ncap_usd: 1\nmax_drop: 1\n",
		"an unknown field": "suite: s\nmodel: m\ncases: 1\ncost_per_case_usd: 0.1\ncap_usd: 1\nmax_drop: 0.1\njudge: x\n",
	} {
		p := filepath.Join(t.TempDir(), "eval.yaml")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))

		_, err := Load(p)
		require.Error(t, err, name)
	}
}

// A run whose estimate is over the cap is not started.
func TestARunOverItsCapIsNotStarted(t *testing.T) {
	t.Parallel()

	c := config()
	require.NoError(t, c.Preflight())

	c.CostPerCase = 0.003
	require.ErrorIs(t, c.Preflight(), ErrOverCap, "37 cases at $0.003 is $0.111")
}

// The model's column gives the cases, the passes and promptfoo's cost; another model's, or none,
// is no result.
func TestTheResultIsTheModelsColumn(t *testing.T) {
	t.Parallel()

	columns := []byte(`{"glm-5.3-flash": {"n": 37, "passed": 30, "pass_rate": 0.8108, "cost": 0.2},
		"mimo-v2.6-flash": {"n": 37, "passed": 35, "pass_rate": 0.9459, "cost": 0.049, "latency_ms_mean": 9000}}`)

	r, err := FromColumns(columns, model)
	require.NoError(t, err)
	assert.Equal(t, Result{Model: model, Cases: 37, Passed: 35, PassRate: 0.9459, CostUSD: 0.049}, r)

	_, err = FromColumns([]byte(`{"glm-5.3-flash": {"n": 37}}`), model)
	require.ErrorIs(t, err, ErrNoColumn)

	_, err = FromColumns([]byte(`{"mimo-v2.6-flash": {"n": 0}}`), model)
	require.ErrorIs(t, err, ErrNoColumn, "a column with no case measured nothing")

	_, err = FromColumns([]byte(`No evaluations found.`), model)
	require.Error(t, err)
}

// The true cost is the model's column of eval-cli's cost report, when it has one.
func TestTheTrueCostIsTheCostReports(t *testing.T) {
	t.Parallel()

	usd, ok := TrueCost([]byte(`{"eval_id": "e1", "columns": {"mimo-v2.6-flash": {"cost_usd": 0.0531, "calls": 74}}}`), model)
	require.True(t, ok)
	assert.InDelta(t, 0.0531, usd, 1e-9)

	_, ok = TrueCost([]byte(`{"eval_id": "e1", "error": "no decodable run ids in this eval's traces"}`), model)
	assert.False(t, ok)

	_, ok = TrueCost([]byte(`{"columns": {"mimo-v2.6-flash": {"calls": 74}}}`), model)
	assert.False(t, ok, "no cost_usd is no cost, not a free run")
}

// A night fails when part of the suite ran, when it cost more than its cap, or when its pass rate
// fell more than the max drop under the previous night's; a smaller drop, or another model's
// previous night, is no problem.
func TestANightIsHeldToItsCasesItsCapAndThePreviousNight(t *testing.T) {
	t.Parallel()

	c := config()
	last := &Result{Model: model, Cases: 37, Passed: 35, PassRate: 0.9459, Recorded: time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)}

	assert.Empty(t, c.Check(Result{Model: model, Cases: 37, Passed: 33, PassRate: 0.8919, CostUSD: 0.05}, last), "a drop of 5 points")
	assert.Empty(t, c.Check(Result{Model: model, Cases: 37, PassRate: 0.5, CostUSD: 0.05}, nil), "the first night has nothing to compare")
	assert.Empty(t, c.Check(Result{Model: "other", Cases: 37, PassRate: 0.5, CostUSD: 0.05}, last), "another model's night is not compared")

	assert.Equal(t, []string{
		"only 30 of the 37 cases of mcp-tools ran",
		"the run cost $0.120, over its cap of $0.10",
		"mimo-v2.6-flash's pass rate fell from 95% (2026-10-06) to 70%, more than 10 points",
	}, c.Check(Result{Model: model, Cases: 30, Passed: 21, PassRate: 0.70, CostUSD: 0.12}, last))
}

// The summary puts the night next to the previous one.
func TestTheSummaryShowsThePreviousNight(t *testing.T) {
	t.Parallel()

	r := Result{Suite: suite, Model: model, Cases: 37, Passed: 33, PassRate: 0.8919, CostUSD: 0.0531}
	assert.Equal(t, "mcp-tools on mimo-v2.6-flash: 33/37 passed (89%), $0.053", Summary(r, nil))

	last := &Result{Cases: 37, Passed: 35, PassRate: 0.9459, Recorded: time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)}
	assert.Equal(t, "mcp-tools on mimo-v2.6-flash: 33/37 passed (89%), $0.053; previous night 35/37 (95%, 2026-10-06)", Summary(r, last))
}

// A result written reads back.
func TestAResultReadsBack(t *testing.T) {
	t.Parallel()

	r := Result{Suite: suite, Model: model, Head: "abc1234", Cases: 37, Passed: 35, PassRate: 0.9459, CostUSD: 0.05, Recorded: time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)}
	p := filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, r.Write(p))

	got, err := Read(p)
	require.NoError(t, err)
	assert.Equal(t, r, got)
}
