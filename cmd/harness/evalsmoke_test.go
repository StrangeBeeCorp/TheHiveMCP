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
	evalFlag   = "--eval"
	evalConfig = "suite: mcp-tools\nmodel: mimo-v2.6-flash\ncases: 37\ncost_per_case_usd: 0.0015\ncap_usd: 0.10\nmax_drop: 0.10\n"
	// fakeUV stands in for `uv run eval-cli`: it records its arguments in $UV_CALLS, exits
	// $UV_RUN_EXIT for eval run, and prints $UV_COLUMNS and $UV_COST for the reports.
	fakeUV = `#!/bin/sh
echo "$*" >> "$UV_CALLS"
case "$3 $4" in
  "eval run") echo "running the suite"; exit "${UV_RUN_EXIT:-0}" ;;
  "eval columns") printf '%s' "$UV_COLUMNS" ;;
  "eval cost") printf '%s' "$UV_COST" ;;
esac
`
	mimoColumns = `{"mimo-v2.6-flash": {"n": 37, "passed": 35, "pass_rate": 0.9459, "cost": 0.049}}`
	mimoCost    = `{"columns": {"mimo-v2.6-flash": {"cost_usd": 0.0531}}}`
)

// evalCheckout is a repository with gates/eval.yaml, and a TheHiveMCPEval checkout, with a fake uv
// first on PATH. It sets the environment, so its tests do not run in parallel.
func evalCheckout(t *testing.T, columns, cost string) (root, eval, calls string) {
	t.Helper()

	root = gittest.Repo(t, map[string]string{evalsmoke.Path: evalConfig})
	eval = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(eval, "pyproject.toml"), []byte("[project]\n"), 0o600))

	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "uv"), []byte(fakeUV), 0o700)) // #nosec G306 -- the test's own fake uv, executable
	calls = filepath.Join(t.TempDir(), "calls")

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("UV_CALLS", calls)
	t.Setenv("UV_COLUMNS", columns)
	t.Setenv("UV_COST", cost)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	t.Setenv("THEHIVEMCP_EVAL", "")

	return root, eval, calls
}

// The night runs the suite against this checkout on the model, then reads its columns and its
// true cost, writes the result, and passes within the cap and the previous night's pass rate.
//
//nolint:paralleltest // evalCheckout sets PATH
func TestTheEvalSmokeRunsTheSuiteAndHoldsItsResult(t *testing.T) {
	root, eval, calls := evalCheckout(t, mimoColumns, mimoCost)
	out := filepath.Join(t.TempDir(), "result.json")

	var log bytes.Buffer

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, "--out", out}, &log)
	require.NoError(t, err, log.String())

	recorded, err := os.ReadFile(calls) // #nosec G304 -- the test's own record
	require.NoError(t, err)
	assert.Equal(t, "run eval-cli eval run mcp-tools --local "+root+" --model mimo-v2.6-flash\nrun eval-cli eval columns\nrun eval-cli eval cost\n", string(recorded))

	r, err := evalsmoke.Read(out)
	require.NoError(t, err)
	assert.Equal(t, 35, r.Passed)
	assert.InDelta(t, 0.0531, r.CostUSD, 1e-9, "the true cost, not promptfoo's")
	assert.Len(t, r.Head, 40)
	assert.Contains(t, log.String(), "mcp-tools on mimo-v2.6-flash: 35/37 passed (95%), $0.053")
}

// Failing cases make eval-cli exit non-zero; the columns still say how the night went, and a pass
// rate that fell more than the max drop under the previous night's fails it.
func TestANightThatFellUnderThePreviousOneFails(t *testing.T) {
	root, eval, _ := evalCheckout(t, `{"mimo-v2.6-flash": {"n": 37, "passed": 26, "pass_rate": 0.7027, "cost": 0.05}}`, mimoCost)
	t.Setenv("UV_RUN_EXIT", "100")

	previous := filepath.Join(t.TempDir(), "previous.json")
	require.NoError(t, evalsmoke.Result{Model: "mimo-v2.6-flash", Cases: 37, Passed: 35, PassRate: 0.9459, Recorded: time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)}.Write(previous))

	var log bytes.Buffer

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval, "--previous", previous}, &log)
	require.ErrorIs(t, err, errEvalSmoke)
	assert.Contains(t, log.String(), "previous night 35/37 (95%, 2026-10-06)")
	assert.Contains(t, log.String(), "pass rate fell from 95% (2026-10-06) to 70%")
}

// A run that leaves no column for the model measured nothing: an error, with eval-cli's output.
func TestARunWithNoResultForTheModelIsAnError(t *testing.T) {
	root, eval, _ := evalCheckout(t, `{"glm-5.3-flash": {"n": 37, "passed": 30}}`, "{}")
	t.Setenv("UV_RUN_EXIT", "1")

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &bytes.Buffer{})
	require.ErrorIs(t, err, evalsmoke.ErrNoColumn)
	assert.Contains(t, err.Error(), "running the suite")
}

// With no checkout, no uv or no OpenRouter key, the smoke cannot run: never a pass, and no call.
func TestAnEvalSmokeThatCannotRunSaysWhy(t *testing.T) {
	root, eval, calls := evalCheckout(t, mimoColumns, mimoCost)

	err := env{root: root}.evalSmoke(t.Context(), nil, &bytes.Buffer{})
	require.ErrorIs(t, err, errCannotRun)
	assert.Contains(t, err.Error(), "THEHIVEMCP_EVAL")

	err = env{root: root}.evalSmoke(t.Context(), []string{evalFlag, t.TempDir()}, &bytes.Buffer{})
	require.ErrorIs(t, err, errCannotRun, "a directory that is no TheHiveMCPEval checkout")

	t.Setenv("OPENROUTER_API_KEY", "")

	err = env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &bytes.Buffer{})
	require.ErrorIs(t, err, errCannotRun, "the Linux image has no keychain: the key comes from the environment")

	t.Setenv("PATH", t.TempDir())

	err = env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &bytes.Buffer{})
	require.ErrorIs(t, err, errCannotRun)
	assert.NoFileExists(t, calls, "nothing ran")
}

// A cap under the estimate stops the night before it starts: nothing is spent.
//
//nolint:paralleltest // evalCheckout sets PATH
func TestANightOverItsCapIsNotStarted(t *testing.T) {
	root, eval, calls := evalCheckout(t, mimoColumns, mimoCost)
	gittest.Write(t, root, map[string]string{evalsmoke.Path: "suite: mcp-tools\nmodel: mimo-v2.6-flash\ncases: 37\ncost_per_case_usd: 0.0015\ncap_usd: 0.05\nmax_drop: 0.10\n"})

	err := env{root: root}.evalSmoke(t.Context(), []string{evalFlag, eval}, &bytes.Buffer{})
	require.ErrorIs(t, err, evalsmoke.ErrOverCap)
	assert.NoFileExists(t, calls)
}
