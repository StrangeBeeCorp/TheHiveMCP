package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evalsmoke"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

const (
	evalFlag     = "--eval"
	previousFlag = "--previous"
	outFlag      = "--out"
	evalConfig   = "suite: mcp-tools\nmodel: mimo-v2.6-flash\ncases: 37\ncost_per_case_usd: 0.0015\ncap_usd: 0.10\nmax_drop: 0.10\n"
	// fakeUV stands in for `uv run eval-cli`, as TheHiveMCPEval answers: it records its arguments
	// in $UV_CALLS; eval run prints a line, writes $UV_SUMMARY to its --summary-json path (nothing
	// when it is empty, as when eval-cli died before writing it) and exits $UV_RUN_EXIT; eval cost
	// prints $UV_COST_<PHASE> for the read or write phase's evaluation, exiting 1 when it is empty.
	fakeUV = `#!/bin/sh
echo "$*" >> "$UV_CALLS"
case "$3 $4" in
  "eval run")
    echo "running the suite"
    [ -z "$UV_SUMMARY" ] || printf '%s' "$UV_SUMMARY" > "${11}"
    exit "${UV_RUN_EXIT:-0}" ;;
  "eval cost")
    phase=$(case "$5" in eval-Rd*) echo READ ;; eval-Wr*) echo WRITE ;; esac)
    eval "out=\$UV_COST_$phase"; [ -n "$out" ] || exit 1; printf '%s' "$out" ;;
esac
`
	readID    = "eval-Rd1-2026-10-06T02:20:01"
	writeID   = "eval-Wr1-2026-10-06T02:31:44"
	summary   = `{"schema": 1, "suites": ["mcp-tools"], "model": "mimo-v2.6-flash", "exit_code": 100, "status": "tests_failed", "failed_step": null, "eval_ids": ["` + readID + `", "` + writeID + `"], "evaluations": [{"eval_id": "` + readID + `", "total": 20, "passed": 19, "failed": 1, "errors": 0}, {"eval_id": "` + writeID + `", "total": 17, "passed": 16, "failed": 1, "errors": 0}]}`
	readCost  = `{"columns": {"mimo-v2.6-flash": {"cost_usd": 0.0281}}, "unattributed": {"cost_usd": 0.004}}`
	writeCost = `{"columns": {"mimo-v2.6-flash": {"cost_usd": 0.025}}}`
	runCall   = "run eval-cli eval run mcp-tools --local "
)

// evalCheckout is a repository with gates/eval.yaml, and a TheHiveMCPEval checkout, with a fake uv
// first on PATH answering for a night of two phases. It sets the environment, so its tests do not
// run in parallel.
func evalCheckout(t *testing.T) (root, eval, calls string) {
	t.Helper()

	root = gittest.Repo(t, map[string]string{evalsmoke.Path: evalConfig})
	eval = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(eval, "pyproject.toml"), []byte("[project]\n"), 0o600))

	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "uv"), []byte(fakeUV), 0o700)) // #nosec G306 -- the test's own fake uv, executable
	calls = filepath.Join(t.TempDir(), "calls")

	for k, v := range map[string]string{
		"PATH": bin + string(os.PathListSeparator) + os.Getenv("PATH"), "UV_CALLS": calls,
		"UV_SUMMARY": summary, "UV_COST_READ": readCost, "UV_COST_WRITE": writeCost,
		"OPENROUTER_API_KEY": "sk-or-test", "THEHIVEMCP_EVAL": "", "UV_RUN_EXIT": "100",
	} {
		t.Setenv(k, v)
	}

	return root, eval, calls
}

// writePrevious writes a previous night's result and returns its path.
func writePrevious(t *testing.T, ok bool, rate float64) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), "previous.json")
	require.NoError(t, evalsmoke.Result{Model: "mimo-v2.6-flash", Cases: 37, Passed: int(rate * 37), PassRate: rate, OK: ok, Recorded: time.Date(2026, 10, 5, 2, 0, 0, 0, time.UTC)}.Write(p))

	return p
}

// The night runs the suite against this checkout on the model, reads the run's summary (the read
// and write phases' evaluations, cases failing included: exit 100), adds each one's true cost,
// writes the result marked as passed next to the summary, and passes within the cap and the
// previous night's pass rate.
//
//nolint:paralleltest // evalCheckout sets PATH
func TestTheEvalSmokeRunsTheSuiteAndHoldsItsResult(t *testing.T) {
	root, eval, calls := evalCheckout(t)
	out := filepath.Join(t.TempDir(), "eval", "result.json")

	var log bytes.Buffer

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, outFlag, out, previousFlag, writePrevious(t, true, 0.9459)}, &log)
	require.NoError(t, err, log.String())

	summaryPath := filepath.Join(filepath.Dir(out), summaryName)
	assert.Equal(t, runCall+root+" --model mimo-v2.6-flash --summary-json "+summaryPath+"\n"+
		"run eval-cli eval cost "+readID+"\nrun eval-cli eval cost "+writeID+"\n", readCalls(t, calls))
	assert.FileExists(t, summaryPath, "the run's summary is kept with the night")

	r, err := evalsmoke.Read(out)
	require.NoError(t, err)
	assert.Equal(t, []string{readID, writeID}, r.Evals)
	assert.Equal(t, 37, r.Cases)
	assert.Equal(t, 35, r.Passed)
	assert.InDelta(t, 0.0531, r.CostUSD, 1e-9, "the model's true costs, not the judge's unattributed calls")
	assert.True(t, r.OK)
	assert.Len(t, r.Head, 40)
	assert.Contains(t, log.String(), "mcp-tools on mimo-v2.6-flash: 35/37 passed (95%), $0.053; previous night 34/37 (95%, 2026-10-05)")
}

// A pass rate that fell more than the max drop under the previous night's fails the night. The
// night is still written, marked as failed: it is no baseline for the next.
func TestANightThatFellUnderThePreviousOneFails(t *testing.T) {
	root, eval, _ := evalCheckout(t)
	t.Setenv("UV_SUMMARY", `{"model": "mimo-v2.6-flash", "exit_code": 100, "status": "tests_failed", "eval_ids": ["`+readID+`", "`+writeID+`"], "evaluations": [{"total": 20, "passed": 19}, {"total": 17, "passed": 7}]}`)

	out := filepath.Join(t.TempDir(), "result.json")

	var log bytes.Buffer

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, outFlag, out, previousFlag, writePrevious(t, true, 0.9459)}, &log)
	require.ErrorIs(t, err, errEvalSmoke)
	assert.Contains(t, log.String(), "pass rate fell from 95% (2026-10-05) to 70%")

	r, err := evalsmoke.Read(out)
	require.NoError(t, err)
	assert.False(t, r.OK, "a night that fell is kept, marked as failed")
}

// A previous night that failed is no baseline; none (the first night) is no error; a corrupt one
// is.
//
//nolint:paralleltest // evalCheckout sets PATH
func TestThePreviousNightIsTheLastOneThatPassed(t *testing.T) {
	root, eval, _ := evalCheckout(t)

	var log bytes.Buffer

	require.NoError(t, env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, previousFlag, writePrevious(t, false, 0.99)}, &log))
	assert.Contains(t, log.String(), "the previous night (2026-10-05) did not pass: not compared with")
	assert.NotContains(t, log.String(), "previous night 36/37")

	log.Reset()
	require.NoError(t, env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, previousFlag, filepath.Join(t.TempDir(), "absent.json")}, &log))
	assert.Contains(t, log.String(), "no previous night to compare with")

	corrupt := filepath.Join(t.TempDir(), "previous.json")
	require.NoError(t, os.WriteFile(corrupt, []byte("{"), 0o600))
	require.Error(t, env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, previousFlag, corrupt}, &bytes.Buffer{}))
}

// A run that did not complete measured nothing: the night fails, naming the step eval-cli says
// failed, with its output, reads no cost and writes no result.
func TestARunThatDidNotCompleteFailsNamingItsStep(t *testing.T) {
	root, eval, calls := evalCheckout(t)
	t.Setenv("UV_RUN_EXIT", "3")
	t.Setenv("UV_SUMMARY", `{"model": "mimo-v2.6-flash", "exit_code": 3, "status": "error", "failed_step": "make mcp-key", "eval_ids": [], "evaluations": []}`)

	out := filepath.Join(t.TempDir(), "result.json")

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, outFlag, out}, &bytes.Buffer{})
	require.ErrorIs(t, err, evalsmoke.ErrRunBroke)
	assert.Contains(t, err.Error(), `"make mcp-key"`)
	assert.Contains(t, err.Error(), "running the suite", "eval-cli's output")
	assert.NotContains(t, readCalls(t, calls), "eval cost")
	assert.NoFileExists(t, out)
}

// eval-cli's exit 2 is a usage error, or CI without an OpenRouter key: the night cannot run here,
// never a pass and never a failure of main.
func TestARunEvalCLIRefusedCannotRun(t *testing.T) {
	root, eval, _ := evalCheckout(t)
	t.Setenv("UV_RUN_EXIT", "2")
	t.Setenv("UV_SUMMARY", `{"model": "mimo-v2.6-flash", "exit_code": 2, "status": "error", "failed_step": "OPENROUTER_API_KEY", "eval_ids": [], "evaluations": []}`)

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &bytes.Buffer{})
	require.ErrorIs(t, err, errCannotRun)
}

// A run that left no summary, one on another model, or one with no evaluation measured nothing:
// an error, with eval-cli's output. A summary an earlier night left in the same place is not read
// as this one's.
func TestARunWithNoResultForTheModelIsAnError(t *testing.T) {
	root, eval, _ := evalCheckout(t)
	out := filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(out), summaryName), []byte(summary), 0o600))

	t.Setenv("UV_SUMMARY", "")

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, outFlag, out}, &bytes.Buffer{})
	require.ErrorIs(t, err, errNoEval, "the earlier night's summary was not read")
	assert.Contains(t, err.Error(), "running the suite")

	for name, s := range map[string]string{
		"another model": `{"model": "glm-5.3-flash", "status": "passed", "eval_ids": ["e1"], "evaluations": [{"total": 37, "passed": 37}]}`,
		"no evaluation": `{"model": "mimo-v2.6-flash", "status": "passed", "eval_ids": [], "evaluations": []}`,
		"zero cases":    `{"model": "mimo-v2.6-flash", "status": "passed", "eval_ids": ["e1"], "evaluations": [{"total": 0}]}`,
		"not JSON":      `No evaluations found.`,
	} {
		t.Setenv("UV_SUMMARY", s)
		require.Error(t, env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &bytes.Buffer{}), name)
	}
}

// Cases that got no answer measured nothing: the night fails, whatever its pass rate.
func TestCasesWithNoAnswerFailTheNight(t *testing.T) {
	root, eval, _ := evalCheckout(t)
	t.Setenv("UV_SUMMARY", `{"model": "mimo-v2.6-flash", "exit_code": 100, "status": "tests_failed", "eval_ids": ["`+readID+`", "`+writeID+`"], "evaluations": [{"total": 20, "passed": 19, "errors": 1}, {"total": 17, "passed": 17}]}`)

	var log bytes.Buffer

	require.ErrorIs(t, env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &log), errEvalSmoke)
	assert.Contains(t, log.String(), "1 case(s) got no answer")
}

// The true cost that eval-cli cannot give, or a true cost over the cap, fails the night: the cap is
// checked on what the night cost, never on promptfoo's estimate.
func TestANightsTrueCostIsHeldToTheCap(t *testing.T) {
	root, eval, _ := evalCheckout(t)
	t.Setenv("UV_COST_WRITE", `{"eval_id": "eval-Wr2", "error": "no decodable run ids in this eval's traces"}`)

	var log bytes.Buffer

	require.ErrorIs(t, env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &log), errEvalSmoke)
	assert.Contains(t, log.String(), "could not attribute the run's true cost")

	t.Setenv("UV_COST_WRITE", "")
	require.ErrorIs(t, env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &bytes.Buffer{}), errEvalSmoke, "eval cost failing")

	t.Setenv("UV_COST_WRITE", `{"columns": {"mimo-v2.6-flash": {"cost_usd": 0.09}}}`)
	log.Reset()
	require.ErrorIs(t, env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &log), errEvalSmoke)
	assert.Contains(t, log.String(), "the run cost $0.118, over its cap of $0.10")
}

// With no checkout, no uv or no OpenRouter key outside macOS, the smoke cannot run: never a
// pass, and nothing runs. On macOS the eval CLI reads the key from the keychain.
func TestAnEvalSmokeThatCannotRunSaysWhy(t *testing.T) {
	root, eval, calls := evalCheckout(t)

	err := env{root: root}.evalSmoke(t.Context(), nil, &bytes.Buffer{})
	require.ErrorIs(t, err, errCannotRun)
	assert.Contains(t, err.Error(), "THEHIVEMCP_EVAL")

	require.ErrorIs(t, evalReady(t.TempDir(), "linux"), errCannotRun, "a directory that is no TheHiveMCPEval checkout")

	t.Setenv("OPENROUTER_API_KEY", "")
	require.ErrorIs(t, evalReady(eval, "linux"), errCannotRun, "no key on Linux")
	require.NoError(t, evalReady(eval, "darwin"), "on macOS the eval CLI reads the keychain")

	t.Setenv("PATH", t.TempDir())
	require.ErrorIs(t, evalReady(eval, "darwin"), errCannotRun, "no uv")
	assert.NoFileExists(t, calls, "nothing ran")
}

// A cap under the estimate stops the night before it starts: nothing is spent.
//
//nolint:paralleltest // evalCheckout sets PATH
func TestANightOverItsCapIsNotStarted(t *testing.T) {
	root, eval, calls := evalCheckout(t)
	gittest.Write(t, root, map[string]string{evalsmoke.Path: "suite: mcp-tools\nmodel: mimo-v2.6-flash\ncases: 37\ncost_per_case_usd: 0.0015\ncap_usd: 0.05\nmax_drop: 0.10\n"})

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &bytes.Buffer{})
	require.ErrorIs(t, err, evalsmoke.ErrOverCap)
	assert.NoFileExists(t, calls)
}
