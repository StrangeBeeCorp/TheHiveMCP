package execute_automation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
)

// The suite's analyzer, run by TheHive on an observable without a cortex-id (the
// server's default Cortex), then waited for: the job ends in Success. Only the
// compose stack's Cortex has that analyzer.
func TestRunAnalyzerOnAnObservableThenWaitForItsReport(t *testing.T) { //nolint:paralleltest // integration test: testutils.Parallel
	testutils.Parallel(t)
	testutils.RequireCortex(t)

	if testutils.SharedMode() {
		t.Skip("the suite's analyzer is enabled only in the compose stack's Cortex")
	}

	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	testCase := testutils.CreateCaseWithTLP(t, hiveClient, "Analyzer run", 2)
	observableID := createObservableWithTLP(t, hiveClient, testCase.UnderscoreId, "198.51.100.7", 2)

	analyzers, httpResp, err := hiveClient.CortexAPI.ListAnalyzersByType(testutils.GetAuthContext(t), "ip").Execute()
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}

	require.NoError(t, err)

	analyzerID := ""

	for _, analyzer := range analyzers {
		if analyzer.Name == testutils.TestCortexAnalyzer {
			analyzerID = analyzer.Id
		}
	}

	require.NotEmpty(t, analyzerID, "TheHive lists the stack's analyzer for an ip: %v", analyzers)

	run := testutils.StructuredData(t, testutils.CallToolOK(t, mcpClient, toolNameExecuteAutomation, map[string]any{
		argOperation:    execute_automation.OperationRunAnalyzer,
		"analyzer-id":   analyzerID,
		"observable-id": observableID,
	}))

	job, ok := run["job"].(map[string]any)
	require.True(t, ok, "run-analyzer answers the created job: %v", run)

	jobID, ok := job["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, jobID)

	waited := testutils.StructuredData(t, testutils.CallToolOK(t, mcpClient, toolNameExecuteAutomation, map[string]any{
		argOperation:      execute_automation.OperationWaitForJobs,
		"job-ids":         []string{jobID},
		"timeout-seconds": 90,
	}))

	jobs, ok := waited["jobs"].([]any)
	require.True(t, ok, "wait-for-jobs answers the jobs: %v", waited)
	require.Len(t, jobs, 1)

	finished, ok := jobs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Success", finished["status"], "the stack's analyzer answers success: %v", waited)
}
