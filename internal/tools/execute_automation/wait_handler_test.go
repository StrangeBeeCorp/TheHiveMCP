package execute_automation

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils/fakecortex"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// scopedPermissions filters execute-automation, so every job is scope-checked.
func scopedPermissions(t *testing.T) *permissions.Config {
	t.Helper()

	perms, err := permissions.ParseYAML([]byte(`version: "1.0"
permissions:
  tools:
    execute-automation:
      allowed: true
      filters:
        _lte: {_field: "tlp", _value: 2}
`))
	require.NoError(t, err)

	return perms
}

func waitWith(t *testing.T, fake *fakecortex.Jobs, perms *permissions.Config, jobIDs []string, timeoutSecs int) (ExecuteAutomationResult, error) {
	t.Helper()

	ctx := context.WithValue(context.Background(), types.HiveClientCtxKey, fake.Client(t))
	ctx = utils.AddPermissionsToContext(ctx, perms)
	tool := &ExecuteAutomationTool{pollInterval: 10 * time.Millisecond}

	return tool.Handle(ctx, mcp.CallToolRequest{}, ExecuteAutomationParams{
		Operation: OperationWaitForJobs, JobIDs: jobIDs, TimeoutSecs: timeoutSecs,
	})
}

func waitForJobs(t *testing.T, replies map[string][]fakecortex.Reply, jobIDs []string, timeoutSecs int) (ExecuteAutomationResult, error) {
	t.Helper()

	return waitWith(t, fakecortex.NewJobs(replies), permissions.LoadAdminForTesting(), jobIDs, timeoutSecs)
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
	assert.Contains(t, string(wait.Message), "2 of 3 job(s) finished. 1 could not be read: see jobErrors.")
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

// A TheHive that never answers must not be reported as jobs still running,
// which would send the model into a loop of long calls against a dead server.
func TestWaitForJobsFailsWhenTheHiveNeverAnswers(t *testing.T) {
	t.Parallel()

	_, err := waitForJobs(t, map[string][]fakecortex.Reply{
		"~1": {fakecortex.Hang()},
		"~2": {fakecortex.Hang()},
	}, []string{"~1", "~2"}, 1)
	require.ErrorContains(t, err, "did not answer within 1s")
}

func TestWaitForJobsFailsWhenNoJobCanBeRead(t *testing.T) {
	t.Parallel()

	_, err := waitForJobs(t, map[string][]fakecortex.Reply{}, []string{"~1", "~2"}, 5)
	require.ErrorContains(t, err, "failed to get the status of any of the 2 jobs")
}

// Regression: the scope check ran for every job before the wait, so with
// permission filters one mistyped or out-of-scope ID failed the whole call.
func TestWaitForJobsReportsAJobOutsideTheScopeWithoutFailingTheOthers(t *testing.T) {
	t.Parallel()

	fake := fakecortex.NewJobs(map[string][]fakecortex.Reply{
		"~1": {fakecortex.Status(statusSuccess)},
		"~2": {fakecortex.Status(statusSuccess)},
	}).OutOfScope("~2")

	result, err := waitWith(t, fake, scopedPermissions(t), []string{"~1", "~2"}, 5)
	require.NoError(t, err)

	wait := result.WaitForJobsResult
	require.NotNil(t, wait)
	assert.Equal(t, []string{"~1"}, jobIDsOf(wait))
	require.Len(t, wait.Failed, 1)
	assert.Equal(t, "~2", wait.Failed[0].JobID)
	assert.Contains(t, wait.Failed[0].Error, "not within the scope permitted")
	assert.Zero(t, fake.Reads("~2"), "a job outside the scope is never read")
}

// Full Cortex reports run to tens of kilobytes; twenty of them overflow a
// client's context, so the wait returns the verdict and nothing else.
func TestWaitForJobsReturnsTheVerdictOnly(t *testing.T) {
	t.Parallel()

	result, err := waitForJobs(t, map[string][]fakecortex.Reply{"~1": {fakecortex.Status(statusSuccess)}}, []string{"~1"}, 5)
	require.NoError(t, err)

	wait := result.WaitForJobsResult
	require.NotNil(t, wait)
	require.Len(t, wait.Jobs, 1)

	report := wait.Jobs[0].Report
	assert.Contains(t, report, "summary")
	assert.Contains(t, report, "success")
	assert.NotContains(t, report, "full")
	assert.NotContains(t, report, "artifacts")
	assert.Contains(t, string(wait.Message), "get-job-status returns a job's full report")
}
