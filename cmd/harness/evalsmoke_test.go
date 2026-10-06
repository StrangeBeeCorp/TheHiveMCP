package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evalsmoke"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	evalFlag     = "--eval"
	previousFlag = "--previous"
	evalConfig   = "suite: mcp-tools\nmodel: mimo-v2.6-flash\ncases: 37\ncost_per_case_usd: 0.0015\ncap_usd: 0.10\nmax_drop: 0.10\n"
	// fakeUV stands in for `uv run eval-cli`, as TheHiveMCPEval prints: it records its arguments in
	// $UV_CALLS; eval list shows the read and write phases' evaluations of every eval run so far
	// (none before the first); eval columns and eval cost print $UV_COLUMNS_<PHASE> and $UV_COST_<PHASE> (exiting 1 when
	// it is empty); eval run exits $UV_RUN_EXIT.
	fakeUV = `#!/bin/sh
echo "$*" >> "$UV_CALLS"
phase=$(case "$5" in eval-Rd*) echo READ ;; eval-Wr*) echo WRITE ;; esac)
case "$3 $4" in
  "eval list")
    n=$(cat "$UV_CALLS.runs" 2>/dev/null || echo 0)
    [ "$n" -gt 0 ] || { echo "No evaluations found."; exit 0; }
    printf 'Eval ID\n-----\n'
    i=1; while [ "$i" -le "$n" ]; do
      printf 'eval-Rd%s-2026-10-06T02:20:01  2026-10-06 04:20 CEST  20\neval-Wr%s-2026-10-06T02:31:44  2026-10-06 04:31 CEST  17\n' "$i" "$i"
      i=$((i+1))
    done ;;
  "eval run") echo $(( $(cat "$UV_CALLS.runs" 2>/dev/null || echo 0) + 1 )) > "$UV_CALLS.runs"; echo "running the suite"; exit "${UV_RUN_EXIT:-0}" ;;
  "eval columns") eval "out=\$UV_COLUMNS_$phase"; [ -n "$out" ] || exit 1; printf '%s' "$out" ;;
  "eval cost") eval "out=\$UV_COST_$phase"; [ -n "$out" ] || exit 1; printf '%s' "$out" ;;
esac
`
	readColumns  = `{"eval_id": "eval-Rd1-2026-10-06T02:20:01", "split": null, "columns": {"mimo-v2.6-flash": {"n": 20, "passed": 19, "pass_rate": 0.95, "cost": 0.03}}}`
	writeColumns = `{"eval_id": "eval-Wr2-2026-10-06T02:31:44", "split": null, "columns": {"mimo-v2.6-flash": {"n": 17, "passed": 16, "pass_rate": 0.9412, "cost": 0.03}}}`
	readCost     = `{"eval_id": "eval-Rd1-2026-10-06T02:20:01", "columns": {"mimo-v2.6-flash": {"cost_usd": 0.0281}}}`
	writeCost    = `{"eval_id": "eval-Wr2-2026-10-06T02:31:44", "columns": {"mimo-v2.6-flash": {"cost_usd": 0.025}}}`
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
		"UV_COLUMNS_READ": readColumns, "UV_COLUMNS_WRITE": writeColumns, "UV_COST_READ": readCost, "UV_COST_WRITE": writeCost,
		"OPENROUTER_API_KEY": "sk-or-test", "THEHIVEMCP_EVAL": "", "UV_RUN_EXIT": "0",
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

// The night runs the suite against this checkout on the model, reads the two evaluations the
// run created (the read and write phases), sums their columns and true costs, writes the result
// marked as passed, and passes within the cap and the previous night's pass rate.
//
//nolint:paralleltest // evalCheckout sets PATH
func TestTheEvalSmokeRunsTheSuiteAndHoldsItsResult(t *testing.T) {
	root, eval, calls := evalCheckout(t)
	out := filepath.Join(t.TempDir(), "eval", "result.json")

	var log bytes.Buffer

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, "--out", out, previousFlag, writePrevious(t, true, 0.9459)}, &log)
	require.NoError(t, err, log.String())

	recorded, err := os.ReadFile(calls) // #nosec G304 -- the test's own record
	require.NoError(t, err)
	assert.Equal(t, "run eval-cli eval list\nrun eval-cli eval run mcp-tools --local "+root+" --model mimo-v2.6-flash\nrun eval-cli eval list\n"+
		"run eval-cli eval columns eval-Rd1-2026-10-06T02:20:01\nrun eval-cli eval cost eval-Rd1-2026-10-06T02:20:01\n"+
		"run eval-cli eval columns eval-Wr1-2026-10-06T02:31:44\nrun eval-cli eval cost eval-Wr1-2026-10-06T02:31:44\n", string(recorded))

	r, err := evalsmoke.Read(out)
	require.NoError(t, err)
	assert.Equal(t, 37, r.Cases)
	assert.Equal(t, 35, r.Passed)
	assert.InDelta(t, 0.0531, r.CostUSD, 1e-9, "the true costs, not promptfoo's")
	assert.True(t, r.OK)
	assert.Len(t, r.Head, 40)
	assert.Contains(t, log.String(), "mcp-tools on mimo-v2.6-flash: 35/37 passed (95%), $0.053; previous night 34/37 (95%, 2026-10-05)")
}

// Failing cases make eval-cli exit non-zero; the evaluations still say how the night went, and a
// pass rate that fell more than the max drop under the previous night's fails it. The night is
// still written, marked as failed: it is no baseline for the next.
func TestANightThatFellUnderThePreviousOneFails(t *testing.T) {
	root, eval, _ := evalCheckout(t)
	t.Setenv("UV_RUN_EXIT", "100")
	t.Setenv("UV_COLUMNS_WRITE", `{"eval_id": "eval-Wr2", "columns": {"mimo-v2.6-flash": {"n": 17, "passed": 7, "pass_rate": 0.41}}}`)

	out := filepath.Join(t.TempDir(), "result.json")

	var log bytes.Buffer

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, "--out", out, previousFlag, writePrevious(t, true, 0.9459)}, &log)
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

// A run that created no evaluation, or whose evaluation has no column for the model, measured
// nothing: an error, with eval-cli's output.
func TestARunWithNoResultForTheModelIsAnError(t *testing.T) {
	root, eval, _ := evalCheckout(t)
	t.Setenv("UV_COLUMNS_READ", `{"eval_id": "eval-Rd1", "columns": {"glm-5.3-flash": {"n": 20, "passed": 15}}}`)

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &bytes.Buffer{})
	require.ErrorIs(t, err, evalsmoke.ErrNoColumn)
	assert.Contains(t, err.Error(), "running the suite")
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
