package manage_test

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// scopedPermissionsYAML restricts manage-entities to entities with TLP <= 2
// (DL-6004 filter enforcement tests).
const scopedPermissionsYAML = `version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
    manage-entities:
      allowed: true
      filters:
        _lte:
          _field: "tlp"
          _value: 2
    execute-automation:
      allowed: true
    get-resource:
      allowed: true
  analyzers:
    mode: "allow_list"
    allowed: ["*"]
  responders:
    mode: "allow_list"
    allowed: ["*"]
`

func scopedManageClient(t *testing.T) *thehive.APIClient {
	t.Helper()
	return testutils.SetupTestWithCleanup(t)
}

func createAlertWithTLP(t *testing.T, hiveClient *thehive.APIClient, sourceRef string, tlp int32) *thehive.OutputAlert {
	t.Helper()

	authContext := testutils.GetAuthContext(t)
	testAlert := testutils.MockInputAlert()
	testAlert.Title = "Alert " + sourceRef
	testAlert.SourceRef = sourceRef
	testAlert.Tlp = &tlp

	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdAlert)

	return createdAlert
}

func requireScopeDenied(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	testutils.RequireScopeDenied(t, result, "not within the scope")
}

// Out-of-scope update is denied without mutating; in-scope update succeeds.
func TestManageScopeUpdateDeniedOutOfScope(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, permsPath)
	authContext := testutils.GetAuthContext(t)

	outOfScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "Out of scope case", 3)
	inScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "In scope case", 2)

	updateRequest := func(caseID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: testToolName,
				Arguments: map[string]any{
					testArgOperation:  testOpUpdate,
					testArgEntityType: types.EntityTypeCase,
					testArgEntityIDs:  []string{caseID},
					testArgEntityData: map[string]any{testFieldTitle: "Title changed by MCP"},
				},
			},
		}
	}

	result, err := mcpClient.CallTool(t.Context(), updateRequest(outOfScopeCase.UnderscoreId))
	require.NoError(t, err)
	requireScopeDenied(t, result)

	fetchedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, outOfScopeCase.UnderscoreId).Execute()
	require.NoError(t, err)
	require.Equal(t, "Out of scope case", fetchedCase.Title, "out-of-scope case must not be mutated")

	result, err = mcpClient.CallTool(t.Context(), updateRequest(inScopeCase.UnderscoreId))
	require.NoError(t, err)
	require.False(t, result.IsError, "update of an in-scope entity must succeed")

	fetchedCase, _, err = hiveClient.CaseAPI.GetCase(authContext, inScopeCase.UnderscoreId).Execute()
	require.NoError(t, err)
	require.Equal(t, "Title changed by MCP", fetchedCase.Title)
}

// A batch update is denied entirely if any entity is out of scope.
func TestManageScopeBatchUpdateDeniedWhenAnyOutOfScope(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, permsPath)
	authContext := testutils.GetAuthContext(t)

	inScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "In scope batch case", 2)
	outOfScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "Out of scope batch case", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpUpdate,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{inScopeCase.UnderscoreId, outOfScopeCase.UnderscoreId},
				testArgEntityData: map[string]any{testFieldTitle: "Batch title"},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)

	// Neither case was mutated: the batch is denied before any update runs
	for _, caseID := range []string{inScopeCase.UnderscoreId, outOfScopeCase.UnderscoreId} {
		fetchedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, caseID).Execute()
		require.NoError(t, err)
		require.NotEqual(t, "Batch title", fetchedCase.Title)
	}
}

// Out-of-scope deletes are denied and the entity survives.
func TestManageScopeDeleteDeniedOutOfScope(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, permsPath)
	authContext := testutils.GetAuthContext(t)

	outOfScopeAlert := createAlertWithTLP(t, hiveClient, "scope-delete-001", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpDelete,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityIDs:  []string{outOfScopeAlert.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)

	fetchedAlert, _, err := hiveClient.AlertAPI.GetAlert(authContext, outOfScopeAlert.UnderscoreId).Execute()
	require.NoError(t, err)
	require.Equal(t, outOfScopeAlert.UnderscoreId, fetchedAlert.UnderscoreId)
}

// Out-of-scope comments are denied; in-scope comments succeed.
func TestManageScopeCommentDeniedOutOfScope(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, permsPath)

	outOfScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "Out of scope comment case", 3)
	inScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "In scope comment case", 2)

	commentRequest := func(caseID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: testToolName,
				Arguments: map[string]any{
					testArgOperation:  testOpComment,
					testArgEntityType: types.EntityTypeCase,
					testArgEntityIDs:  []string{caseID},
					testOpComment:     "Scope enforcement test comment",
				},
			},
		}
	}

	result, err := mcpClient.CallTool(t.Context(), commentRequest(outOfScopeCase.UnderscoreId))
	require.NoError(t, err)
	requireScopeDenied(t, result)

	result, err = mcpClient.CallTool(t.Context(), commentRequest(inScopeCase.UnderscoreId))
	require.NoError(t, err)
	require.False(t, result.IsError, "comment on an in-scope entity must succeed")
}

// Child creation (task) inside an out-of-scope parent case is denied.
func TestManageScopeCreateChildDeniedOutOfScopeParent(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, permsPath)

	outOfScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "Out of scope parent case", 3)
	inScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "In scope parent case", 2)

	createTaskRequest := func(caseID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: testToolName,
				Arguments: map[string]any{
					testArgOperation:  testOpCreate,
					testArgEntityType: types.EntityTypeTask,
					testArgEntityIDs:  []string{caseID},
					testArgEntityData: map[string]any{testFieldTitle: "Scope test task"},
				},
			},
		}
	}

	result, err := mcpClient.CallTool(t.Context(), createTaskRequest(outOfScopeCase.UnderscoreId))
	require.NoError(t, err)
	requireScopeDenied(t, result)

	result, err = mcpClient.CallTool(t.Context(), createTaskRequest(inScopeCase.UnderscoreId))
	require.NoError(t, err)
	require.False(t, result.IsError, "task creation in an in-scope case must succeed")
}

// Case-or-alert parent resolution: observable in an in-scope alert succeeds,
// in an out-of-scope alert is denied.
func TestManageScopeCreateObservableInScopeAlertParent(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, permsPath)

	inScopeAlert := createAlertWithTLP(t, hiveClient, "scope-obs-in-001", 2)
	outOfScopeAlert := createAlertWithTLP(t, hiveClient, "scope-obs-out-001", 3)

	createObservableRequest := func(parentID, data string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: testToolName,
				Arguments: map[string]any{
					testArgOperation:  testOpCreate,
					testArgEntityType: types.EntityTypeObservable,
					testArgEntityIDs:  []string{parentID},
					testArgEntityData: map[string]any{
						testFieldDataType: "ip",
						testFieldData:     data,
						testFieldMessage:  "scope test observable",
					},
				},
			},
		}
	}

	result, err := mcpClient.CallTool(t.Context(), createObservableRequest(inScopeAlert.UnderscoreId, "10.10.10.10"))
	require.NoError(t, err)
	require.False(t, result.IsError, "observable creation in an in-scope alert must succeed")

	result, err = mcpClient.CallTool(t.Context(), createObservableRequest(outOfScopeAlert.UnderscoreId, "10.10.10.11"))
	require.NoError(t, err)
	requireScopeDenied(t, result)
}

// Promoting an out-of-scope alert is denied.
func TestManageScopePromoteDeniedOutOfScope(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, permsPath)

	outOfScopeAlert := createAlertWithTLP(t, hiveClient, "scope-promote-001", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpPromote,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityIDs:  []string{outOfScopeAlert.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)
}

// A merge spanning an out-of-scope entity is denied.
func TestManageScopeMergeDeniedWhenAnyCaseOutOfScope(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, permsPath)

	inScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "In scope merge case", 2)
	outOfScopeCase := testutils.CreateCaseWithTLP(t, hiveClient, "Out of scope merge case", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpMerge,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{inScopeCase.UnderscoreId, outOfScopeCase.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)
}

// Without configured filters (admin), operations a filter would exclude proceed.
func TestManageScopeNoFiltersBackwardCompatible(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := scopedManageClient(t)
	mcpClient := testutils.GetMCPTestClient(t)
	authContext := testutils.GetAuthContext(t)

	highTLPCase := testutils.CreateCaseWithTLP(t, hiveClient, "High TLP case", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpUpdate,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{highTLPCase.UnderscoreId},
				testArgEntityData: map[string]any{testFieldTitle: "Updated without filters"},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.False(t, result.IsError, "without configured filters the update must proceed")

	fetchedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, highTLPCase.UnderscoreId).Execute()
	require.NoError(t, err)
	require.Equal(t, "Updated without filters", fetchedCase.Title)
}
