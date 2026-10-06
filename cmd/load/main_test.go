package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/load"
)

// cmd/load makes the runs against the stub only: the thehive run is the integration suite's, and
// measuring the stub in its name would overwrite TheHive's baseline with the stub's numbers.
//
//nolint:paralleltest // t.Chdir: the command reads gates/load.yaml from the checkout's root
func TestARunAgainstTheHiveIsRefused(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))

	err := run(t.Context(), []string{"--run", "thehive", "--write-baseline"}, &bytes.Buffer{})
	require.ErrorIs(t, err, errNotStub)
	assert.Contains(t, err.Error(), "TestLoadAgainstTheHive")
}

// A run over its budget writes no baseline, even when asked: a bad measurement does not become
// what later runs are compared with. Within its budget, it does.
//
//nolint:paralleltest // t.Chdir: the baseline is written under the working directory
func TestABaselineIsWrittenOnlyWithinBudget(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(load.BaselineDir, 0o750))

	slow := load.Result{Run: "stub", P95ms: 300, RSSMiB: []float64{20}}

	var out bytes.Buffer

	err := report(slow, load.Budget{P95ms: 200}, true, &out)
	require.ErrorIs(t, err, errOverBudget)
	assert.Contains(t, err.Error(), "no baseline written")
	assert.NoFileExists(t, load.Baseline(".", "stub"))

	require.NoError(t, report(slow, load.Budget{P95ms: 400}, true, &out))
	assert.FileExists(t, load.Baseline(".", "stub"))
	assert.Contains(t, out.String(), "stub within its budget")

	require.NoError(t, report(slow, load.Budget{P95ms: 400}, false, &out))
	assert.Contains(t, out.String(), "stub against its baseline of", "a run compares itself with the baseline there")
}
