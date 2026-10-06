package triage

import (
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
	lintGate = "lint"
	lintLog  = "lint.log"
	runURL   = "https://github.com/o/r/actions/runs/1"
	checks   = "checks"
	fuzzGate = "fuzz"
)

func issue(n int, key string) Issue {
	return Issue{Number: n, Title: "t", Body: "<!-- nightly-finding: " + key + " -->\nbody"}
}

// A failing gate opens a finding that names its key; failing again comments on it, and passing
// closes it. A skipped gate, or a finding whose gate did not run tonight, is left as it is.
func TestTheNightOpensCommentsAndClosesFindings(t *testing.T) {
	t.Parallel()

	outcomes := []Outcome{
		{Shard: checks, Gate: lintGate, Verdict: runner.Fail, Summary: "lint failed"},
		{Shard: checks, Gate: fuzzGate, Verdict: runner.Fail, Summary: "FuzzFilterDSL broke"},
		{Shard: checks, Gate: "race", Verdict: runner.Pass},
		{Shard: checks, Gate: "bench", Verdict: runner.Skipped},
		{Shard: "integration-5.5", Gate: "integration", Verdict: runner.Unavailable, Summary: "no docker"},
	}
	open := []Issue{issue(7, "checks/fuzz"), issue(8, "checks/race"), issue(9, "checks/bench"), issue(10, "checks/gone"), {Number: 11, Body: "a person's issue"}}

	actions := Plan(outcomes, open, runURL)
	require.Len(t, actions, 4)

	assert.Equal(t, Comment, actions[0].Kind)
	assert.Equal(t, 7, actions[0].Issue)
	assert.Contains(t, actions[0].Body, "Still failing in "+runURL)
	assert.Contains(t, actions[0].Body, "FuzzFilterDSL broke")

	assert.Equal(t, Open, actions[1].Kind)
	assert.Equal(t, "nightly: lint fails (checks)", actions[1].Title)
	assert.Equal(t, "checks/lint", Issue{Body: actions[1].Body}.Key(), "the new issue names its finding")
	assert.Contains(t, actions[1].Body, "make gate ID=lint CADENCE=nightly")

	assert.Equal(t, Action{Kind: Close, Issue: 8, Body: "Passes again in " + runURL + "."}, actions[2])

	assert.Equal(t, Open, actions[3].Kind)
	assert.Equal(t, "nightly: integration cannot run (integration-5.5)", actions[3].Title)
	assert.Contains(t, actions[3].Body, "unable to run")
}

// A gate that ran twice in a shard is judged by its worst run.
func TestAGateIsJudgedByItsWorstRun(t *testing.T) {
	t.Parallel()

	actions := Plan([]Outcome{
		{Shard: checks, Gate: fuzzGate, Verdict: runner.Pass},
		{Shard: checks, Gate: fuzzGate, Verdict: runner.Fail, Summary: "second run"},
		{Shard: checks, Gate: fuzzGate, Verdict: runner.Unavailable},
	}, []Issue{issue(3, "checks/fuzz")}, runURL)

	require.Len(t, actions, 1)
	assert.Equal(t, Comment, actions[0].Kind)
	assert.Contains(t, actions[0].Body, "second run")
}

// A log is data: no backtick run inside it closes the fence that quotes it.
func TestALogCannotCloseItsFence(t *testing.T) {
	t.Parallel()

	log := "before\n````\n# Injected heading\nafter"
	actions := Plan([]Outcome{{Shard: checks, Gate: lintGate, Verdict: runner.Fail, Summary: "s", Log: log}}, nil, runURL)
	require.Len(t, actions, 1)

	body := actions[0].Body
	assert.Contains(t, body, "`````text\n"+log+"\n`````\n")
}

// writeShard writes one shard's artifact: a run directory with its results.json and its logs.
func writeShard(t *testing.T, dir, shard string, results []runner.Result, logs map[string]string) {
	t.Helper()

	run := filepath.Join(dir, shard, "nightly-20261006-020000")
	require.NoError(t, os.MkdirAll(run, 0o750))

	data, err := json.Marshal(runner.Record{Cadence: "nightly", Results: results})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(run, "results.json"), data, 0o600))

	for name, content := range logs {
		require.NoError(t, os.WriteFile(filepath.Join(run, name), []byte(content), 0o600))
	}
}

// Read finds each shard's results and the tail of each gate's log, though the runner recorded the
// log's path on another machine; an expected shard with no results is a finding of its own, and a
// shard that ran closes its own.
func TestReadFindsEachShardsResultsAndLogs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	long := strings.Repeat("line\n", 100) + "the last line\n"
	writeShard(t, dir, checks, []runner.Result{
		{Gate: lintGate, Verdict: runner.Fail, Summary: "lint failed", Log: "/home/runner/work/.cache/harness/runs/x/lint.log"},
		{Gate: "race", Verdict: runner.Pass},
	}, map[string]string{lintLog: long})

	// An artifact that is no shard, as the fuzz inputs, holds no results.json: it adds nothing.
	inputs := filepath.Join(dir, "fuzz-inputs", "internal", "utils", "testdata", "fuzz", "FuzzFilterDSL")
	require.NoError(t, os.MkdirAll(inputs, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(inputs, "a1b2"), []byte("go test fuzz v1\n"), 0o600))

	outcomes, err := Read(dir, []string{checks, "mutation"})
	require.NoError(t, err)
	require.Len(t, outcomes, 4)

	assert.Equal(t, Outcome{Shard: checks, Gate: anyGate, Verdict: runner.Pass}, outcomes[0])
	assert.Equal(t, lintGate, outcomes[1].Gate)
	assert.True(t, strings.HasSuffix(outcomes[1].Log, "the last line"), outcomes[1].Log)
	assert.Len(t, strings.Split(outcomes[1].Log, "\n"), logLines)
	assert.Empty(t, outcomes[2].Log, "a gate without a log quotes none")
	assert.Equal(t, Outcome{Shard: "mutation", Gate: anyGate, Verdict: runner.Unavailable, Summary: "no results.json in the shard's artifact"}, outcomes[3])

	actions := Plan(outcomes, []Issue{issue(4, "checks/*")}, runURL)
	assert.Equal(t, Close, actions[0].Kind, "the shard ran again: its own finding closes")
	assert.Equal(t, "nightly: the mutation shard records no result", actions[len(actions)-1].Title)
}

// A log is quoted at most logBytes long, its start cut: one long line as well as many.
func TestALongLogIsCutToItsEnd(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeShard(t, dir, checks, []runner.Result{{Gate: lintGate, Verdict: runner.Fail, Log: lintLog}},
		map[string]string{lintLog: strings.Repeat("x", 10*logBytes) + "the end"})

	outcomes, err := Read(dir, nil)
	require.NoError(t, err)
	require.Len(t, outcomes, 2)

	tail := outcomes[1].Log
	assert.True(t, strings.HasPrefix(tail, "…"), "the cut is shown")
	assert.True(t, strings.HasSuffix(tail, "the end"))
	assert.Len(t, tail, len("…")+logBytes)
}

// The artifact is read through an os.Root: a log or a results.json that is a symlink out of the
// shard quotes nothing from outside it.
func TestASymlinkCannotLeadOutOfTheShard(t *testing.T) {
	t.Parallel()

	secret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secret, []byte("a runner secret"), 0o600))

	dir := t.TempDir()
	writeShard(t, dir, checks, []runner.Result{{Gate: lintGate, Verdict: runner.Fail, Log: lintLog}}, nil)
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, checks, "nightly-20261006-020000", lintLog)))

	outcomes, err := Read(dir, nil)
	require.NoError(t, err)
	require.Len(t, outcomes, 2)
	assert.Empty(t, outcomes[1].Log, "the log outside the shard is not quoted")

	other := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(other, checks), 0o750))
	require.NoError(t, os.Symlink(secret, filepath.Join(other, checks, "results.json")))

	_, err = Read(other, nil)
	require.Error(t, err, "a results.json outside the shard is not read")
	assert.NotContains(t, err.Error(), "a runner secret")
}

// With no shard at all and none expected there is nothing to triage: that is an error, never a
// quiet night.
func TestAnEmptyNightIsAnError(t *testing.T) {
	t.Parallel()

	_, err := Read(filepath.Join(t.TempDir(), "absent"), nil)
	require.ErrorIs(t, err, ErrNoShards)
}

// A results.json that does not parse fails the triage rather than dropping the shard.
func TestABrokenRecordFailsTheTriage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, checks), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, checks, "results.json"), []byte("{"), 0o600))

	_, err := Read(dir, nil)
	require.ErrorContains(t, err, "shard checks")
}
