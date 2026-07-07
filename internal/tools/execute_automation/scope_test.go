package execute_automation_test

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

const (
	toolNameExecuteAutomation = "execute-automation"
	argOperation              = "operation"
	argEntityType             = "entity-type"
	argEntityID               = "entity-id"
)

// scopedPermissionsYAML restricts execute-automation to TLP <= 2 but allows every
// analyzer/responder, so the entity scope check is the deciding gate (DL-6004).
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

func createObservableWithTLP(t *testing.T, hiveClient *thehive.APIClient, caseID, data string, tlp int32) string {
	t.Helper()

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	observable := thehive.NewInputCreateObservable("ip")
	observable.SetData(thehive.StringAsInputObservableData(new(data)))
	observable.SetMessage("scope test observable")
	observable.SetTlp(tlp)

	created, _, err := hiveClient.ObservableAPI.CreateObservableInCase(authContext, caseID).InputCreateObservable(*observable).Execute()
	require.NoError(t, err)
	require.NotEmpty(t, created)

	return created[0].UnderscoreId
}

func requireScopeDenied(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	testutils.RequireScopeDenied(t, result, "not within the scope")
}

func TestExecuteAutomationScopeRunResponderDeniedOutOfScope(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	outOfScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "Out of scope responder target", 3)
	inScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "In scope responder target", 2)

	runResponderRequest := func(caseID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolNameExecuteAutomation,
				Arguments: map[string]any{
					argOperation:   "run-responder",
					"responder-id": "TestResponder_1_0",
					argEntityType:  types.EntityTypeCase,
					argEntityID:    caseID,
				},
			},
		}
	}

	result, err := mcpClient.CallTool(t.Context(), runResponderRequest(outOfScopeCase.UnderscoreId))
	require.NoError(t, err)
	requireScopeDenied(t, result)

	// In-scope target passes the gate; the Cortex call then fails since no Cortex is connected.
	result, err = mcpClient.CallTool(t.Context(), runResponderRequest(inScopeCase.UnderscoreId))
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	text := textContent.Text
	require.NotContains(t, text, "not within the scope")
	require.Contains(t, text, "failed to execute responder")
}

func TestExecuteAutomationScopeRunAnalyzerDeniedOutOfScope(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	// Scope is decided on the observable's own TLP: the parent case is in scope (TLP 2)
	// but the TLP-3 observable is not.
	parentCase := testutils.CreateCaseWithTLP(t, hiveClient, "Case for analyzer scope test", 2)
	outOfScopeObservableID := createObservableWithTLP(t, hiveClient, parentCase.UnderscoreId, "10.20.30.40", 3)
	inScopeObservableID := createObservableWithTLP(t, hiveClient, parentCase.UnderscoreId, "10.20.30.41", 1)

	runAnalyzerRequest := func(observableID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolNameExecuteAutomation,
				Arguments: map[string]any{
					argOperation:    "run-analyzer",
					"analyzer-id":   "TestAnalyzer_1_0",
					"observable-id": observableID,
				},
			},
		}
	}

	result, err := mcpClient.CallTool(t.Context(), runAnalyzerRequest(outOfScopeObservableID))
	require.NoError(t, err)
	requireScopeDenied(t, result)

	// In-scope observable passes the gate; TheHive accepts the analyzer job even without a connected Cortex.
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

	statusResult, err := mcpClient.CallTool(t.Context(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolNameExecuteAutomation,
			Arguments: map[string]any{
				argOperation: "get-job-status",
				"job-id":     jobID,
			},
		},
	})
	require.NoError(t, err)
	require.False(t, statusResult.IsError, "job status for an in-scope target must be readable")
}

func TestExecuteAutomationScopeGetJobStatusDeniedOutOfScope(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	parentCase := testutils.CreateCaseWithTLP(t, hiveClient, "Case for job status scope test", 2)
	outOfScopeObservableID := createObservableWithTLP(t, hiveClient, parentCase.UnderscoreId, "10.20.30.42", 3)

	// Create the job directly through TheHive, bypassing the MCP gate
	inputJob := thehive.NewInputJob("TestAnalyzer_1_0", "local", outOfScopeObservableID)
	job, _, err := hiveClient.CortexAPI.CreateCortexJob(authContext).InputJob(*inputJob).Execute()
	require.NoError(t, err)
	require.NotNil(t, job)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolNameExecuteAutomation,
			Arguments: map[string]any{
				argOperation: "get-job-status",
				"job-id":     job.GetUnderscoreId(),
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)
}

func TestExecuteAutomationScopeGetActionStatusDeniedOutOfScope(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	outOfScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "Out of scope action status target", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolNameExecuteAutomation,
			Arguments: map[string]any{
				argOperation:  "get-action-status",
				"action-id":   "~999999",
				argEntityType: types.EntityTypeCase,
				argEntityID:   outOfScopeCase.UnderscoreId,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)
}

// No filters means the scope gate is skipped: a high-TLP entity reaches the lookup.
func TestExecuteAutomationScopeNoFiltersBackwardCompatible(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	highTLPCase := testutils.CreateCaseWithTLP(t, hiveClient, "High TLP case without filters", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolNameExecuteAutomation,
			Arguments: map[string]any{
				argOperation:  "get-action-status",
				"action-id":   "~999999",
				argEntityType: types.EntityTypeCase,
				argEntityID:   highTLPCase.UnderscoreId,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	text := textContent.Text
	// The action lookup itself ran (and found nothing); it was not blocked by
	// any scope gate
	require.NotContains(t, text, "not within the scope")
	require.Contains(t, text, "not found for entity")
}
