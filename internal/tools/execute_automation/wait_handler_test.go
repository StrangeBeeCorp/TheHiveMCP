package execute_automation

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils/fakecortex"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func waitForJobs(t *testing.T, replies map[string][]fakecortex.Reply, jobIDs []string, timeoutSecs int) (ExecuteAutomationResult, error) {
	t.Helper()

	ctx := context.WithValue(context.Background(), types.HiveClientCtxKey, fakecortex.NewJobs(replies).Client(t))
	tool := &ExecuteAutomationTool{pollInterval: 10 * time.Millisecond}

	return tool.Handle(ctx, mcp.CallToolRequest{}, ExecuteAutomationParams{
		Operation: OperationWaitForJobs, JobIDs: jobIDs, TimeoutSecs: timeoutSecs,
	})
}

func jobIDsOf(result *WaitForJobsResult) []string {
	ids := make([]string, 0, len(result.Jobs))
	for _, job := range result.Jobs {
		ids = append(ids, job.UnderscoreID)
	}

	return ids
}

// Regression: one failed read returned an error and threw away the reports
// already collected for every other job.
func TestWaitForJobsKeepsTheOtherReportsWhenOneReadFails(t *testing.T) {
	t.Parallel()

	result, err := waitForJobs(t, map[string][]fakecortex.Reply{
		"~1": {fakecortex.Status(statusInProgress), fakecortex.Status(statusSuccess)},
		"~2": {fakecortex.Unavailable(), fakecortex.Status(statusSuccess)},
		// "~3" is unknown to the fake: a 404, which is not asked again.
	}, []string{"~1", "~2", "~3"}, 5)
	require.NoError(t, err)

	wait := result.WaitForJobsResult
	require.NotNil(t, wait)
	assert.Equal(t, []string{"~1", "~2"}, jobIDsOf(wait), "a transient error is retried, and the other jobs keep their reports")
	assert.Empty(t, wait.Pending, "a job TheHive refuses is not waited for until the timeout")
	require.Len(t, wait.Failed, 1)
	assert.Equal(t, "~3", wait.Failed[0].JobID)
	assert.Contains(t, string(wait.Message), "jobErrors")
}

// Regression: the reads had no deadline, so a stalled TheHive held the call
// past the timeout meant to keep it inside the client's own.
func TestWaitForJobsReturnsByTheTimeoutWhenTheHiveStalls(t *testing.T) {
	t.Parallel()

	started := time.Now()
	result, err := waitForJobs(t, map[string][]fakecortex.Reply{
		"~1": {fakecortex.Status(statusSuccess)},
		"~2": {fakecortex.Status(statusInProgress), fakecortex.Hang()},
	}, []string{"~1", "~2"}, 1)
	require.NoError(t, err)

	assert.Less(t, time.Since(started), 3*time.Second)

	wait := result.WaitForJobsResult
	require.NotNil(t, wait)
	assert.Equal(t, []string{"~1", "~2"}, jobIDsOf(wait))
	assert.Equal(t, []string{"~2"}, wait.Pending, "a job cut off by the deadline stays pending as last seen")
	assert.Empty(t, wait.Failed)
}

func TestWaitForJobsFailsWhenNoJobCanBeRead(t *testing.T) {
	t.Parallel()

	_, err := waitForJobs(t, map[string][]fakecortex.Reply{}, []string{"~1", "~2"}, 5)
	require.ErrorContains(t, err, "failed to get the status of any of the 2 jobs")
}
