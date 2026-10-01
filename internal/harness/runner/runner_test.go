package runner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	stop  = "stop"
	clean = "clean"
	dirty = "dirty"
)

func gate(id, runs string, budget int) registry.Gate {
	return registry.Gate{ID: id, Runs: runs, Cadence: []string{stop}, Suites: []string{"*"}, BudgetS: budget, Blocks: "x"}
}

func TestRunGivesEachGateItsVerdict(t *testing.T) {
	t.Parallel()

	reg := &registry.Registry{Scopes: map[string]string{stop: "--changed HEAD"}}
	gates := []registry.Gate{
		gate("ok", "echo scope {scope}; echo all good", 10),
		gate("bad", "echo broken; exit 1", 10),
		gate("absent", "echo no docker here; exit 75", 10),
		gate("slow", "sleep 5", 1),
		{ID: clean, Builtin: clean, BudgetS: 10},
		{ID: dirty, Builtin: dirty, BudgetS: 10},
		{ID: "needs-docker", Runs: "true", Docker: true, BudgetS: 10},
	}
	logDir := t.TempDir()
	results, err := runner.Run(context.Background(), reg, gates, runner.Options{
		Root: t.TempDir(), Cadence: stop, LogDir: logDir,
		Builtin: func(_ context.Context, name string) ([]string, error) {
			if name == dirty {
				return []string{"a.go: too long", "b.go: too long"}, nil
			}

			return nil, nil
		},
		DockerUp: func(context.Context) bool { return false },
	})
	require.NoError(t, err)

	verdicts := map[string]runner.Verdict{}
	for _, r := range results {
		verdicts[r.Gate] = r.Verdict
	}

	assert.Equal(t, map[string]runner.Verdict{
		"ok": runner.Pass, "bad": runner.Fail, "absent": runner.Unavailable, "slow": runner.Fail,
		clean: runner.Pass, dirty: runner.Fail, "needs-docker": runner.Unavailable,
	}, verdicts)
	assert.Equal(t, "all good", results[0].Summary)
	assert.Equal(t, "over its 1 s budget", results[3].Summary)
	assert.Equal(t, "2 finding(s): a.go: too long", results[5].Summary)

	log, err := os.ReadFile(filepath.Join(logDir, "ok.log")) // #nosec G304 -- a log the test just wrote
	require.NoError(t, err)
	assert.Contains(t, string(log), "scope --changed HEAD", "the cadence's scope replaces {scope}")
	assert.True(t, runner.Failed(results))
	assert.False(t, runner.Failed(results[2:3]), "a limit is not a failure")

	var out bytes.Buffer
	runner.Print(&out, results, false)
	assert.Contains(t, out.String(), "--- bad (last 40 lines of")
	assert.Contains(t, out.String(), "broken")
}

func TestRecordIsWrittenAsJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rec := runner.Record{Cadence: "ci", Verdict: runner.Pass, Results: []runner.Result{{Gate: "lint", Verdict: runner.Pass}}}
	p, err := rec.Write(dir)
	require.NoError(t, err)
	data, err := os.ReadFile(p) // #nosec G304 -- the record the test just wrote
	require.NoError(t, err)

	var back runner.Record
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, rec.Results, back.Results)
}
