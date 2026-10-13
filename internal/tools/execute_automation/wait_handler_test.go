package execute_automation

import (
	"context"
	"strings"
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
	assert.False(t, wait.Failed[0].Retryable, "a 404 will not change")
	assert.Contains(t, string(wait.Message), "2 of 3 job(s) finished. 1 could not be read: see jobErrors")
	assert.NotContains(t, string(wait.Message), "retryable", "no failure is retryable, so pendingJobIds holds none of them")
}

// Regression: a job whose last read before the deadline failed transiently was
// dropped from pendingJobIds, so a model following "call again with
// pendingJobIds" never asked for it again and lost its verdict.
func TestWaitForJobsKeepsARetryableFailurePending(t *testing.T) {
	t.Parallel()

	result, err := waitForJobs(t, map[string][]fakecortex.Reply{
		"~1": {fakecortex.Status(statusSuccess)},
		"~2": {fakecortex.Status(statusInProgress), fakecortex.Unavailable()},
	}, []string{"~1", "~2"}, 1)
	require.NoError(t, err)

	wait := result.WaitForJobsResult
	require.NotNil(t, wait)
	assert.Equal(t, []string{"~2"}, wait.Pending)
	require.Len(t, wait.Failed, 1)
	assert.Equal(t, "~2", wait.Failed[0].JobID)
	assert.True(t, wait.Failed[0].Retryable)
	assert.Contains(t, string(wait.Message), "A retryable one is also in pendingJobIds.")
}

// Regression: a timeout above the maximum was capped silently, so a model that
// asked for 500 s read the shorter wait as the whole one.
func TestWaitForJobsSaysWhenTheTimeoutWasCapped(t *testing.T) {
	t.Parallel()

	params := ExecuteAutomationParams{Operation: OperationWaitForJobs, JobIDs: []string{"~1"}, TimeoutSecs: 500}
	require.NoError(t, (&ExecuteAutomationTool{}).ValidateParams(&params))
	require.Equal(t, MaxWaitSeconds, params.TimeoutSecs)

	fake := fakecortex.NewJobs(map[string][]fakecortex.Reply{"~1": {fakecortex.Status(statusSuccess)}})
	ctx := context.WithValue(context.Background(), types.HiveClientCtxKey, fake.Client(t))
	ctx = utils.AddPermissionsToContext(ctx, permissions.LoadAdminForTesting())

	result, err := (&ExecuteAutomationTool{pollInterval: 10 * time.Millisecond}).Handle(ctx, mcp.CallToolRequest{}, params)
	require.NoError(t, err)
	require.NotNil(t, result.WaitForJobsResult)
	assert.Contains(t, string(result.WaitForJobsResult.Message), "timeout-seconds was capped at the maximum, 120.")

	uncapped, err := waitForJobs(t, map[string][]fakecortex.Reply{"~1": {fakecortex.Status(statusSuccess)}}, []string{"~1"}, 5)
	require.NoError(t, err)
	assert.NotContains(t, string(uncapped.WaitForJobsResult.Message), "capped")

	// When no job can be read, the error says it too.
	unknown := ExecuteAutomationParams{Operation: OperationWaitForJobs, JobIDs: []string{"~9"}, TimeoutSecs: 500}
	require.NoError(t, (&ExecuteAutomationTool{}).ValidateParams(&unknown))

	_, err = (&ExecuteAutomationTool{pollInterval: 10 * time.Millisecond}).Handle(ctx, mcp.CallToolRequest{}, unknown)
	require.ErrorContains(t, err, "failed to get the status of any of the 1 jobs")
	assert.ErrorContains(t, err, "timeout-seconds was capped at the maximum, 120")
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

// A scope check that fails must not stop the wait from reading the jobs after it.
func TestWaitForJobsReadsTheOtherJobsWhenAScopeCheckFails(t *testing.T) {
	t.Parallel()

	fake := fakecortex.NewJobs(map[string][]fakecortex.Reply{
		"~1": {fakecortex.Status(statusSuccess)},
		"~2": {fakecortex.Status(statusSuccess)},
	}).ScopeUnavailable("~1")

	result, err := waitWith(t, fake, scopedPermissions(t), []string{"~1", "~2"}, 1)
	require.NoError(t, err)

	wait := result.WaitForJobsResult
	require.NotNil(t, wait)
	assert.Equal(t, []string{"~2"}, jobIDsOf(wait))
	assert.Equal(t, []string{"~1"}, wait.Pending, "a scope check that failed is worth trying again")
	require.Len(t, wait.Failed, 1)
	assert.Contains(t, wait.Failed[0].Error, "failed to check the job against your permission filters")
	assert.Zero(t, fake.Reads("~1"), "a job whose scope is unknown is not read")
}

// A tool built without a poll interval polls every waitPollInterval, not in a busy loop.
func TestWaitForJobsPollsAtTheDefaultInterval(t *testing.T) {
	t.Parallel()

	fake := fakecortex.NewJobs(map[string][]fakecortex.Reply{
		"~1": {fakecortex.Status(statusInProgress), fakecortex.Status(statusSuccess)},
	})
	ctx := context.WithValue(context.Background(), types.HiveClientCtxKey, fake.Client(t))
	ctx = utils.AddPermissionsToContext(ctx, permissions.LoadAdminForTesting())

	result, err := (&ExecuteAutomationTool{}).Handle(ctx, mcp.CallToolRequest{}, ExecuteAutomationParams{
		Operation: OperationWaitForJobs, JobIDs: []string{"~1"}, TimeoutSecs: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, fake.Reads("~1"), "the next read is due after the 1 s timeout")
	assert.Equal(t, []string{"~1"}, result.WaitForJobsResult.Pending)
}

func TestWaitForJobsMessageCountsTheFinishedJobs(t *testing.T) {
	t.Parallel()

	result, err := waitForJobs(t, map[string][]fakecortex.Reply{
		"~1": {fakecortex.Status(statusSuccess)},
		"~2": {fakecortex.Status(statusSuccess)},
	}, []string{"~1", "~2"}, 5)
	require.NoError(t, err)

	message := string(result.WaitForJobsResult.Message)
	assert.True(t, strings.HasPrefix(message, "2 of 2 job(s) finished. Reports carry"), message)
}
