package execute_automation_test

import (
	"strings"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// harmlessResponder picks a responder applicable to the case that cannot delete
// anything, with the Cortex it belongs to (empty when TheHive tells none).
func harmlessResponder(t *testing.T, hiveClient *thehive.APIClient, caseID string) (thehive.OutputWorker, string, bool) {
	t.Helper()

	responders, httpResp, err := hiveClient.CortexAPI.ListResponders(testutils.GetAuthContext(t), types.EntityTypeCase, caseID).Execute()
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}

	if err != nil {
		return thehive.OutputWorker{}, "", false
	}

	for _, responder := range responders {
		if strings.Contains(strings.ToLower(responder.Name), "delete") {
			continue
		}

		cortexID := ""
		if len(responder.CortexIds) > 0 {
			cortexID = responder.CortexIds[0]
		}

		return responder, cortexID, true
	}

	return thehive.OutputWorker{}, "", false
}

// A responder really run by a TheHive 5.8 or newer: the action it answers (with
// its extraData) and the status it reads back are not failures.
//
// spec: VER-002
func TestRunResponderOnACaseThenReadItsStatus(t *testing.T) { //nolint:paralleltest // integration test: testutils.Parallel
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	testCase := testutils.CreateCaseWithTLP(t, hiveClient, "Responder run", 2)

	responder, cortexID, found := harmlessResponder(t, hiveClient, testCase.UnderscoreId)
	if !found {
		t.Skip("the TheHive under test has no Cortex responder applicable to a case")
	}

	args := map[string]any{
		argOperation:   "run-responder",
		"responder-id": responder.Id,
		argEntityType:  types.EntityTypeCase,
		argEntityID:    testCase.UnderscoreId,
	}
	if cortexID != "" {
		args["cortex-id"] = cortexID
	}

	runResult := testutils.CallToolOK(t, mcpClient, toolNameExecuteAutomation, args)
	run := testutils.StructuredData(t, runResult)

	action, ok := run["action"].(map[string]any)
	require.True(t, ok, "run-responder answers the created action: %v", run)

	actionID, ok := action["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, actionID)

	statusResult, err := mcpClient.CallTool(t.Context(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolNameExecuteAutomation,
			Arguments: map[string]any{
				argOperation:  execute_automation.OperationGetActionStatus,
				argActionID:   actionID,
				argEntityType: types.EntityTypeCase,
				argEntityID:   testCase.UnderscoreId,
			},
		},
	})
	require.NoError(t, err)
	assert.False(t, statusResult.IsError, "get-action-status reads the action TheHive created: %v", statusResult.Content)
}
