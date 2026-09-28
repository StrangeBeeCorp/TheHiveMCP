package execute_automation

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func TestWaitForJobsParamsNormalised(t *testing.T) {
	t.Parallel()

	params := ExecuteAutomationParams{Operation: OperationWaitForJobs, JobIDs: []string{"~1", "~2", "~1"}}
	require.NoError(t, validateWaitForJobsParams(&params))
	require.Equal(t, []string{"~1", "~2"}, params.JobIDs, "a repeated ID is waited for once")
	require.Equal(t, DefaultWaitSeconds, params.TimeoutSecs)

	params = ExecuteAutomationParams{Operation: OperationWaitForJobs, JobIDs: []string{"~1"}, TimeoutSecs: 10 * MaxWaitSeconds}
	require.NoError(t, validateWaitForJobsParams(&params))
	require.Equal(t, MaxWaitSeconds, params.TimeoutSecs, "a long wait is clamped, not refused")
}

func TestWaitForJobsParamsRefused(t *testing.T) {
	t.Parallel()

	tooMany := make([]string, MaxWaitJobs+1)
	for i := range tooMany {
		tooMany[i] = string(rune('a' + i))
	}

	for name, params := range map[string]ExecuteAutomationParams{
		"no ids":           {},
		"empty id":         {JobIDs: []string{"~1", ""}},
		"negative timeout": {JobIDs: []string{"~1"}, TimeoutSecs: -1},
		"too many ids":     {JobIDs: tooMany},
	} {
		require.Error(t, validateWaitForJobsParams(&params), name)
	}
}

// TestWaitForJobsResultKeepsReportsUntrusted pins that batching the jobs does
// not bypass the report fence get-job-status applies (DL-6703).
func TestWaitForJobsResultKeepsReportsUntrusted(t *testing.T) {
	t.Parallel()

	done := &thehive.OutputJob{UnderscoreId: "~1", Status: statusSuccess, Report: map[string]any{"status": "injected"}}
	running := &thehive.OutputJob{UnderscoreId: "~2", Status: "InProgress"}

	processed, err := utils.ProcessDatesRecursive(NewWaitForJobsResult([]*thehive.OutputJob{done, running}, []string{"~2"}, 5), true)
	require.NoError(t, err)

	m, ok := processed.(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"~2"}, m["pendingJobIds"])

	jobs, ok := m["jobs"].([]any)
	require.True(t, ok)
	first, ok := jobs[0].(map[string]any)
	require.True(t, ok)
	report, ok := first["report"].(map[string]any)
	require.True(t, ok, "expected report map, got %T", first["report"])
	require.Equal(t, "[UNTRUSTED_DATA]injected[/UNTRUSTED_DATA]", report["status"])
}
