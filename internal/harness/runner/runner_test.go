package runner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	assert.Equal(t, runner.Fail, runner.Overall(results))
	assert.Equal(t, runner.Incomplete, runner.Overall(results[2:3]), "a limit is never a pass")
	assert.Equal(t, runner.Pass, runner.Overall(results[0:1]))

	var out bytes.Buffer
	runner.Print(&out, results)
	assert.Contains(t, out.String(), "--- bad (last 40 lines of")
	assert.Contains(t, out.String(), "broken")
}

func TestAGateComparesWithTheBaseOfTheRun(t *testing.T) {
	t.Setenv("HARNESS_BASE", "HEAD")

	logDir := t.TempDir()
	_, err := runner.Run(context.Background(), &registry.Registry{}, []registry.Gate{gate("base", "echo base=$HARNESS_BASE", 10)},
		runner.Options{Root: t.TempDir(), Cadence: stop, Base: "origin/main", LogDir: logDir})
	require.NoError(t, err)

	log, err := os.ReadFile(filepath.Join(logDir, "base.log")) // #nosec G304 -- a log the test just wrote
	require.NoError(t, err)
	assert.Contains(t, string(log), "base=origin/main", "the run's base wins over one the caller exported")
}

func TestStreamedRunsStillPrintBuiltinFindings(t *testing.T) {
	t.Parallel()

	reg := &registry.Registry{Scopes: map[string]string{stop: ""}}
	gates := []registry.Gate{gate("bad", "echo broken; exit 1", 10), {ID: dirty, Builtin: dirty, BudgetS: 10}}

	var streamed bytes.Buffer

	results, err := runner.Run(context.Background(), reg, gates, runner.Options{
		Root: t.TempDir(), Cadence: stop, LogDir: t.TempDir(), Stream: true, Out: &streamed,
		Builtin: func(context.Context, string) ([]string, error) {
			return []string{"a.go: too long", "b.go: too long"}, nil
		},
	})
	require.NoError(t, err)
	assert.Contains(t, streamed.String(), "broken", "a command's output is copied as it runs")

	var out bytes.Buffer
	runner.Print(&out, results)
	assert.NotContains(t, out.String(), "--- bad", "streamed output is not printed twice")
	assert.Contains(t, out.String(), "b.go: too long", "a builtin's findings after the first reach the CI log")
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

// A built-in gate's findings are printed whole, a command's output only to its last 40 lines.
func TestPrintShowsEveryFindingOfABuiltin(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	findings := make([]string, 60)
	for i := range findings {
		findings[i] = fmt.Sprintf("file%02d.go: too long", i)
	}

	results, err := runner.Run(context.Background(), &registry.Registry{}, []registry.Gate{{ID: dirty, Builtin: dirty, BudgetS: 10}},
		runner.Options{Root: dir, Cadence: stop, LogDir: dir, Builtin: func(context.Context, string) ([]string, error) { return findings, nil }})
	require.NoError(t, err)

	var out bytes.Buffer
	runner.Print(&out, results)
	assert.Contains(t, out.String(), "file00.go: too long", "the first of 60")
	assert.Contains(t, out.String(), "file59.go: too long")

	assert.Len(t, runner.Tail(results[0].Log, -1), 60)
	assert.Len(t, runner.Tail(results[0].Log, 40), 40)
}
