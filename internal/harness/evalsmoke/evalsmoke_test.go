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

// The summary `eval-cli eval run --summary-json` writes, as #81 of TheHiveMCPEval documents it,
// gives the run's evaluations and the night is their sum: cases, passes and rows with no answer.
func TestTheNightIsTheSumOfTheEvaluationsItsRunCreated(t *testing.T) {
	t.Parallel()

	run, err := ReadRun([]byte(`{"schema": 1, "suites": ["mcp-tools"], "model": "mimo-v2.6-flash", "mcp_version": "local",
		"exit_code": 100, "status": "tests_failed", "failed_step": null, "duration_s": 887.2,
		"eval_ids": ["eval-Rd1-2026-10-06T02:20:01", "eval-Wr2-2026-10-06T02:31:44"],
		"evaluations": [
			{"eval_id": "eval-Rd1-2026-10-06T02:20:01", "description": "MCP Tools (read) [MCP local]", "tests": 20, "total": 20, "passed": 19, "failed": 1, "errors": 0},
			{"eval_id": "eval-Wr2-2026-10-06T02:31:44", "description": "MCP Tools (write) [MCP local]", "tests": 17, "total": 17, "passed": 14, "failed": 2, "errors": 1}]}`))
	require.NoError(t, err)

	night, err := run.Result(model)
	require.NoError(t, err)
	assert.Equal(t, []string{"eval-Rd1-2026-10-06T02:20:01", "eval-Wr2-2026-10-06T02:31:44"}, night.Evals)
	assert.Equal(t, 37, night.Cases)
	assert.Equal(t, 33, night.Passed)
	assert.Equal(t, 1, night.Unanswered)
	assert.InDelta(t, 33.0/37, night.PassRate, 1e-9)

	costs := Result{}.Add(Result{CostUSD: 0.029}).Add(Result{CostUSD: 0.024, CostUnmeasured: true})
	assert.InDelta(t, 0.053, costs.CostUSD, 1e-9)
	assert.True(t, costs.CostUnmeasured, "one evaluation's cost unmeasured leaves the night's unmeasured")
}

// A run that did not complete, one on another model, or one with no case measured nothing.
func TestARunThatMeasuredNothingHasNoResult(t *testing.T) {
	t.Parallel()

	_, err := Run{Model: model, ExitCode: 1, Status: "error", FailedStep: "run the suites in the eval container"}.Result(model)
	require.ErrorIs(t, err, ErrRunBroke)
	assert.Contains(t, err.Error(), "run the suites in the eval container")

	_, err = Run{Model: model, ExitCode: 130, Status: "interrupted"}.Result(model)
	require.ErrorIs(t, err, ErrRunBroke)

	_, err = Run{Model: "glm-5.3-flash", Status: StatusPassed, Evaluations: []Evaluation{{Total: 37, Passed: 37}}}.Result(model)
	require.ErrorIs(t, err, ErrNoEval)

	_, err = Run{Model: model, Status: StatusPassed}.Result(model)
	require.ErrorIs(t, err, ErrNoEval)

	_, err = ReadRun([]byte(`No evaluations found.`))
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

	assert.Equal(t, []string{"74 cases read for the 37 of mcp-tools: evaluations that are not this run's were read"},
		c.Check(Result{Model: model, Cases: 74, PassRate: 0.95, CostUSD: 0.05}, last), "an older night read as this one")

	assert.Equal(t, []string{"eval-cli could not attribute the run's true cost (eval-cli eval cost): the cap was not checked"},
		c.Check(Result{Model: model, Cases: 37, PassRate: 0.95, CostUnmeasured: true}, last), "an unmeasured cost is no pass")

	assert.Equal(t, []string{"2 case(s) got no answer and measured nothing: rerun them before trusting the pass rate"},
		c.Check(Result{Model: model, Cases: 37, Passed: 35, PassRate: 0.9459, Unanswered: 2, CostUSD: 0.05}, last), "rows with no answer are no pass")

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

	r := Result{Suite: suite, Model: model, Head: "abc1234", Evals: []string{"eval-Rd1", "eval-Wr2"}, OK: true, Cases: 37, Passed: 35, PassRate: 0.9459, CostUSD: 0.05, Recorded: time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)}
	p := filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, r.Write(p))

	got, err := Read(p)
	require.NoError(t, err)
	assert.Equal(t, r, got)
}
