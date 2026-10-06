package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	runsFlag = "--runs"
	urlFlag  = "--run-url"
	nightURL = "https://github.com/o/r/actions/runs/9"
	// fakeGH records each call's arguments and stdin in $GH_CALLS, answers issue list with
	// $GH_ISSUES, and fails every other call with $GH_FAIL when it is set.
	fakeGH = `#!/bin/sh
{ echo "ARGS $*"; if [ "$1" = issue ] && [ "$2" != list ] && [ "$2" != close ]; then cat; echo; fi; } >> "$GH_CALLS"
if [ "$1 $2" = "issue list" ]; then printf '%s' "$GH_ISSUES"; exit 0; fi
if [ -n "$GH_FAIL" ]; then echo "$GH_FAIL" >&2; exit 1; fi
`
)

// withFakeGH puts a gh that records its calls first on PATH, answering issue list with issues.
// It sets the environment, so its tests do not run in parallel.
func withFakeGH(t *testing.T, issues string) string {
	t.Helper()

	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(fakeGH), 0o700)) // #nosec G306 -- the test's own fake gh, executable
	calls := filepath.Join(t.TempDir(), "calls")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_CALLS", calls)
	t.Setenv("GH_ISSUES", issues)

	return calls
}

func readCalls(t *testing.T, calls string) string {
	t.Helper()

	data, err := os.ReadFile(calls) // #nosec G304 -- the test's own record
	if err != nil {
		return ""
	}

	return string(data)
}

// nightRuns writes one shard, checks, where lint failed and race passed.
func nightRuns(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	run := filepath.Join(dir, "checks", "nightly-1")
	require.NoError(t, os.MkdirAll(run, 0o750))

	data, err := json.Marshal(runner.Record{Results: []runner.Result{
		{Gate: "lint", Verdict: runner.Fail, Summary: "lint failed"},
		{Gate: "race", Verdict: runner.Pass},
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(run, "results.json"), data, 0o600))

	return dir
}

// The triage files the new failure with the nightly label (made first), closes the finding that
// passes again, and reports a shard that recorded nothing.
//
//nolint:paralleltest // withFakeGH sets PATH
func TestTheTriageFilesAndClosesTheNightsFindings(t *testing.T) {
	calls := withFakeGH(t, `[{"number": 5, "title": "nightly: race fails (checks)", "body": "<!-- nightly-finding: checks/race -->"}]`)

	var out bytes.Buffer

	err := env{root: t.TempDir()}.triageNight(t.Context(), []string{runsFlag, nightRuns(t), urlFlag, nightURL, "--expect", "checks,mutation"}, &out)
	require.NoError(t, err, out.String())

	got := readCalls(t, calls)
	assert.Contains(t, got, "ARGS label create nightly --force")
	assert.Contains(t, got, "ARGS issue create --title nightly: lint fails (checks) --label nightly --body-file -\n<!-- nightly-finding: checks/lint -->")
	assert.Contains(t, got, "ARGS issue close 5 --comment Passes again in "+nightURL+".")
	assert.Contains(t, got, "ARGS issue create --title nightly: the mutation shard records no result")
	assert.Less(t, strings.Index(got, "label create"), strings.Index(got, "issue create"), "the label exists before an issue carries it")
	assert.Contains(t, out.String(), "3 finding(s) changed, 1 open before this night")
}

// A dry run prints the plan and changes nothing.
//
//nolint:paralleltest // withFakeGH sets PATH
func TestADryTriageChangesNothing(t *testing.T) {
	calls := withFakeGH(t, `[]`)

	var out bytes.Buffer

	require.NoError(t, env{root: t.TempDir()}.triageNight(t.Context(), []string{runsFlag, nightRuns(t), urlFlag, nightURL, "--dry-run"}, &out))
	assert.Equal(t, "open: nightly: lint fails (checks)\n", out.String())
	assert.Equal(t, "ARGS issue list --label nightly --state open --limit 500 --json number,title,body\n", readCalls(t, calls))
}

// A finding that still fails gets the night's comment on stdin, and a night that opens nothing
// leaves the label alone.
//
//nolint:paralleltest // withFakeGH sets PATH
func TestTheTriageCommentsOnAFindingThatStillFails(t *testing.T) {
	calls := withFakeGH(t, `[{"number": 6, "title": "nightly: lint fails (checks)", "body": "<!-- nightly-finding: checks/lint -->"}]`)

	var out bytes.Buffer

	require.NoError(t, env{root: t.TempDir()}.triageNight(t.Context(), []string{runsFlag, nightRuns(t), urlFlag, nightURL}, &out), out.String())

	got := readCalls(t, calls)
	assert.Contains(t, got, "ARGS issue comment 6 --body-file -\nStill failing in "+nightURL+".")
	assert.Contains(t, got, "lint failed")
	assert.NotContains(t, got, "label create")
	assert.NotContains(t, got, "issue create")
}

// When gh fails, the triage fails with what gh said: a night is never filed in part silently.
func TestATriageWhoseGHFailsFails(t *testing.T) {
	withFakeGH(t, `[]`)
	t.Setenv("GH_FAIL", "HTTP 403: Resource not accessible by integration")

	err := env{root: t.TempDir()}.triageNight(t.Context(), []string{runsFlag, nightRuns(t), urlFlag, nightURL}, &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gh label create")
	assert.Contains(t, err.Error(), "HTTP 403")
}

// The triage needs its directory and the run's URL.
func TestTheTriageNeedsItsArguments(t *testing.T) {
	t.Parallel()

	err := env{}.triageNight(t.Context(), []string{runsFlag, "x"}, &bytes.Buffer{})

	var usage usageError
	require.ErrorAs(t, err, &usage)
}

// make next shows a blocked agent first, then the open findings, the oldest first.
//
//nolint:paralleltest // withFakeGH sets PATH
func TestNextListsTheOpenFindingsOldestFirst(t *testing.T) {
	withFakeGH(t, `[
		{"number": 9, "title": "nightly: fuzz fails (checks)", "url": "u9", "createdAt": "2026-10-05T02:00:00Z"},
		{"number": 4, "title": "nightly: lint fails (checks)", "url": "u4", "createdAt": "2026-10-01T02:00:00Z"}]`)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".cache", "harness"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, blockedPath), []byte("the coder is red on lint\n"), 0o600))

	var out bytes.Buffer

	require.NoError(t, env{root: root}.next(t.Context(), &out))
	assert.Equal(t, "An agent is blocked (.cache/harness/BLOCKED.md):\n\nthe coder is red on lint\n"+
		"2 open nightly finding(s), the oldest first:\n"+
		"  #4  nightly: lint fails (checks)  (since 2026-10-01)  u4\n"+
		"  #9  nightly: fuzz fails (checks)  (since 2026-10-05)  u9\n", out.String())
}

// Without gh, make next says what it needs.
func TestNextWithoutGHSaysSo(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := env{root: t.TempDir()}.next(t.Context(), &bytes.Buffer{})
	require.ErrorIs(t, err, errNoGH)
}
