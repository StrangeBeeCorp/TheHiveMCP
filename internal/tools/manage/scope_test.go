package manage_test

import (
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
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

func createAlertWithTLP(t *testing.T, hiveClient *thehive.APIClient, sourceRef string, tlp int32) *thehive.OutputAlert {
	t.Helper()

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
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
	require.True(t, result.IsError, "operation on an out-of-scope entity must be denied")
	require.Contains(t, result.Content[0].(mcp.TextContent).Text, "not within the scope")
}

// Out-of-scope update is denied without mutating; in-scope update succeeds.
func TestManageScopeUpdateDeniedOutOfScope(t *testing.T) {
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	outOfScopeCase := createCaseWithTLP(t, hiveClient, "Out of scope case", 3)
	inScopeCase := createCaseWithTLP(t, hiveClient, "In scope case", 2)

	updateRequest := func(caseID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "manage-entities",
				Arguments: map[string]any{
					"operation":   "update",
					"entity-type": types.EntityTypeCase,
					"entity-ids":  []string{caseID},
					"entity-data": map[string]interface{}{"title": "Title changed by MCP"},
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
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	inScopeCase := createCaseWithTLP(t, hiveClient, "In scope batch case", 2)
	outOfScopeCase := createCaseWithTLP(t, hiveClient, "Out of scope batch case", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "update",
				"entity-type": types.EntityTypeCase,
				"entity-ids":  []string{inScopeCase.UnderscoreId, outOfScopeCase.UnderscoreId},
				"entity-data": map[string]interface{}{"title": "Batch title"},
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
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	outOfScopeAlert := createAlertWithTLP(t, hiveClient, "scope-delete-001", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "delete",
				"entity-type": types.EntityTypeAlert,
				"entity-ids":  []string{outOfScopeAlert.UnderscoreId},
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
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	outOfScopeCase := createCaseWithTLP(t, hiveClient, "Out of scope comment case", 3)
	inScopeCase := createCaseWithTLP(t, hiveClient, "In scope comment case", 2)

	commentRequest := func(caseID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "manage-entities",
				Arguments: map[string]any{
					"operation":   "comment",
					"entity-type": types.EntityTypeCase,
					"entity-ids":  []string{caseID},
					"comment":     "Scope enforcement test comment",
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
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	outOfScopeCase := createCaseWithTLP(t, hiveClient, "Out of scope parent case", 3)
	inScopeCase := createCaseWithTLP(t, hiveClient, "In scope parent case", 2)

	createTaskRequest := func(caseID string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "manage-entities",
				Arguments: map[string]any{
					"operation":   "create",
					"entity-type": types.EntityTypeTask,
					"entity-ids":  []string{caseID},
					"entity-data": map[string]interface{}{"title": "Scope test task"},
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
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	inScopeAlert := createAlertWithTLP(t, hiveClient, "scope-obs-in-001", 2)
	outOfScopeAlert := createAlertWithTLP(t, hiveClient, "scope-obs-out-001", 3)

	createObservableRequest := func(parentID, data string) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "manage-entities",
				Arguments: map[string]any{
					"operation":   "create",
					"entity-type": types.EntityTypeObservable,
					"entity-ids":  []string{parentID},
					"entity-data": map[string]interface{}{
						"dataType": "ip",
						"data":     data,
						"message":  "scope test observable",
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
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	outOfScopeAlert := createAlertWithTLP(t, hiveClient, "scope-promote-001", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "promote",
				"entity-type": types.EntityTypeAlert,
				"entity-ids":  []string{outOfScopeAlert.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)
}

// A merge spanning an out-of-scope entity is denied.
func TestManageScopeMergeDeniedWhenAnyCaseOutOfScope(t *testing.T) {
	hiveClient := scopedManageClient(t)
	permsPath := testutils.WritePermissionsFile(t, scopedPermissionsYAML)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, permsPath)

	inScopeCase := createCaseWithTLP(t, hiveClient, "In scope merge case", 2)
	outOfScopeCase := createCaseWithTLP(t, hiveClient, "Out of scope merge case", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "merge",
				"entity-type": types.EntityTypeCase,
				"entity-ids":  []string{inScopeCase.UnderscoreId, outOfScopeCase.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	requireScopeDenied(t, result)
}

// Without configured filters (admin), operations a filter would exclude proceed.
func TestManageScopeNoFiltersBackwardCompatible(t *testing.T) {
	hiveClient := scopedManageClient(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	highTLPCase := createCaseWithTLP(t, hiveClient, "High TLP case", 3)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "update",
				"entity-type": types.EntityTypeCase,
				"entity-ids":  []string{highTLPCase.UnderscoreId},
				"entity-data": map[string]interface{}{"title": "Updated without filters"},
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
