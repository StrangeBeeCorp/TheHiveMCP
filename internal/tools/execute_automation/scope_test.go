package execute_automation_test

import (
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// scopedPermissionsYAML restricts execute-automation to entities with
// TLP <= 2 while allowing every analyzer and responder, so the entity scope
// check is the deciding gate (DL-6004 filter enforcement tests).
const scopedPermissionsYAML = `version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
    manage-entities:
      allowed: true
    execute-automation:
      allowed: true
      filters:
        _lte:
          _field: "tlp"
          _value: 2
    get-resource:
      allowed: true
  analyzers:
    mode: "allow_list"
    allowed: ["*"]
  responders:
    mode: "allow_list"
    allowed: ["*"]
`

func createCaseWithTLP(t *testing.T, hiveClient *thehive.APIClient, title string, tlp int32) *thehive.OutputCase {
	t.Helper()

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = title
	testCase.Tlp = &tlp

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)
	return createdCase
}

func createObservableWithTLP(t *testing.T, hiveClient *thehive.APIClient, caseID, data string, tlp int32) string {
	t.Helper()

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	observable := thehive.NewInputCreateObservable("ip")
	observable.SetData(thehive.StringAsInputObservableData(thehive.PtrString(data)))
	observable.SetMessage("scope test observable")
	observable.SetTlp(tlp)

	created, _, err := hiveClient.ObservableAPI.CreateObservableInCase(authContext, caseID).InputCreateObservable(*observable).Execute()
	require.NoError(t, err)
	require.NotEmpty(t, created)
	return created[0].UnderscoreId
}

func requireScopeDenied(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	require.True(t, result.IsError, "automation against an out-of-scope entity must be denied")
	require.Contains(t, result.Content[0].(mcp.TextContent).Text, "not within the scope")
}

// TestExecuteAutomationScopeRunResponderDeniedOutOfScope verifies that
// running a responder against an out-of-scope entity is denied before any
// Cortex call, while an in-scope target passes the scope gate.
func TestExecuteAutomationScopeRunResponderDeniedOutOfScope(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	outOfScopeCase := createCaseWithTLP(t, hiveClient, "Out of scope responder target", 3)
	inScopeCase := createCaseWithTLP(t, hiveClient, "In scope responder target", 2)

	runResponderRequest := func(caseID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "execute-automation",
				Arguments: map[string]any{
					"operation":    "run-responder",
					"responder-id": "TestResponder_1_0",
					"entity-type":  types.EntityTypeCase,
					"entity-id":    caseID,
				},
			},
		}
	}

	// Out-of-scope target: denied by the scope gate
	result, err := mcpClient.CallTool(t.Context(), runResponderRequest(outOfScopeCase.UnderscoreId))
	require.NoError(t, err)
	requireScopeDenied(t, result)

	// In-scope target: passes the scope gate and reaches TheHive's Cortex
	// API (which fails here because no Cortex instance is connected)
	result, err = mcpClient.CallTool(t.Context(), runResponderRequest(inScopeCase.UnderscoreId))
	require.NoError(t, err)
	require.True(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	require.NotContains(t, text, "not within the scope")
	require.Contains(t, text, "failed to execute responder")
}

// TestExecuteAutomationScopeRunAnalyzerDeniedOutOfScope verifies that running
// an analyzer on an out-of-scope observable is denied before any Cortex call.
func TestExecuteAutomationScopeRunAnalyzerDeniedOutOfScope(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	// The parent case is in scope; the observable itself carries TLP 3 and is
	// therefore outside the configured filter
	parentCase := createCaseWithTLP(t, hiveClient, "Case for analyzer scope test", 2)
	outOfScopeObservableID := createObservableWithTLP(t, hiveClient, parentCase.UnderscoreId, "10.20.30.40", 3)
	inScopeObservableID := createObservableWithTLP(t, hiveClient, parentCase.UnderscoreId, "10.20.30.41", 1)

	runAnalyzerRequest := func(observableID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "execute-automation",
				Arguments: map[string]any{
					"operation":     "run-analyzer",
					"analyzer-id":   "TestAnalyzer_1_0",
					"observable-id": observableID,
				},
			},
		}
	}

	result, err := mcpClient.CallTool(t.Context(), runAnalyzerRequest(outOfScopeObservableID))
	require.NoError(t, err)
	requireScopeDenied(t, result)

	// In-scope observable passes the gate: TheHive accepts the job even
	// without a connected Cortex instance
	result, err = mcpClient.CallTool(t.Context(), runAnalyzerRequest(inScopeObservableID))
	require.NoError(t, err)
	require.False(t, result.IsError, "running an analyzer on an in-scope observable must succeed")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	job, ok := structuredData["job"].(map[string]any)
	require.True(t, ok)
	jobID, ok := job["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, jobID)

	// Reading the status of a job targeting an in-scope observable succeeds
	statusResult, err := mcpClient.CallTool(t.Context(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "execute-automation",
			Arguments: map[string]any{
				"operation": "get-job-status",
				"job-id":    jobID,
			},
		},
	})
	require.NoError(t, err)
	require.False(t, statusResult.IsError, "job status for an in-scope target must be readable")
}

// TestExecuteAutomationScopeGetJobStatusDeniedOutOfScope verifies that
// reading the status (and report) of a job whose target observable is out of
// scope is denied.
func TestExecuteAutomationScopeGetJobStatusDeniedOutOfScope(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	parentCase := createCaseWithTLP(t, hiveClient, "Case for job status scope test", 2)
	outOfScopeObservableID := createObservableWithTLP(t, hiveClient, parentCase.UnderscoreId, "10.20.30.42", 3)

	// Create the job directly through TheHive, bypassing the MCP gate
	inputJob := thehive.NewInputJob("TestAnalyzer_1_0", "local", outOfScopeObservableID)
	job, _, err := hiveClient.CortexAPI.CreateCortexJob(authContext).InputJob(*inputJob).Execute()
	require.NoError(t, err)
	require.NotNil(t, job)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "execute-automation",
			Arguments: map[string]any{
				"operation": "get-job-status",
				"job-id":    job.GetUnderscoreId(),
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)
}

// TestExecuteAutomationScopeGetActionStatusDeniedOutOfScope verifies that
// reading action status for an out-of-scope entity is denied.
func TestExecuteAutomationScopeGetActionStatusDeniedOutOfScope(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	outOfScopeCase := createCaseWithTLP(t, hiveClient, "Out of scope action status target", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "execute-automation",
			Arguments: map[string]any{
				"operation":   "get-action-status",
				"action-id":   "~999999",
				"entity-type": types.EntityTypeCase,
				"entity-id":   outOfScopeCase.UnderscoreId,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)
}

// TestExecuteAutomationScopeNoFiltersBackwardCompatible verifies that without
// configured filters the scope gate does not interfere: get-action-status on
// a high-TLP entity proceeds to the lookup itself.
func TestExecuteAutomationScopeNoFiltersBackwardCompatible(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	highTLPCase := createCaseWithTLP(t, hiveClient, "High TLP case without filters", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "execute-automation",
			Arguments: map[string]any{
				"operation":   "get-action-status",
				"action-id":   "~999999",
				"entity-type": types.EntityTypeCase,
				"entity-id":   highTLPCase.UnderscoreId,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.True(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	// The action lookup itself ran (and found nothing); it was not blocked by
	// any scope gate
	require.NotContains(t, text, "not within the scope")
	require.Contains(t, text, "not found for entity")
}
