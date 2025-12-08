package manage_test

import (
	"fmt"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// TestManageCreateAlert tests creating a new alert via the manage-entities tool
func TestManageCreateAlert(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	alertData := map[string]interface{}{
		"type":        "test-type",
		"source":      "test-source",
		"sourceRef":   "test-create-alert-001",
		"title":       "Test Alert via MCP",
		"description": "This alert was created through the manage-entities tool",
		"severity":    3,
		"tlp":         2,
		"pap":         2,
		"tags":        []string{"test", "automated"},
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "create",
				"entity-type": "alert",
				"entity-data": alertData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "create", structuredData["operation"])
	require.Equal(t, "alert", structuredData["entityType"])

	resultsAlert, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	// Verify the alert was created with correct data
	alertID, ok := resultsAlert["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, alertID)
	require.Equal(t, "Test Alert via MCP", resultsAlert["title"])
	require.Equal(t, float64(3), resultsAlert["severity"])

	// Verify the alert exists in TheHive by fetching it
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	fetchedAlert, _, err := hiveClient.AlertAPI.GetAlert(authContext, alertID).Execute()
	require.NoError(t, err)
	require.Equal(t, "Test Alert via MCP", fetchedAlert.Title)
}

// TestManageUpdateCase tests updating an existing case via the manage-entities tool
func TestManageUpdateCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	// First create a case to update
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Original Case Title"
	severity := int32(2)
	testCase.Severity = &severity
	testCase.Tags = []string{"initial"}

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	// Update the case using manage-entities
	updateData := map[string]interface{}{
		"title":       "Updated Case Title",
		"severity":    4,
		"description": "Updated description through MCP tool",
		"tags":        []string{"initial", "updated"},
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "update",
				"entity-type": "case",
				"entity-ids":  []string{createdCase.UnderscoreId},
				"entity-data": updateData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "update", structuredData["operation"])

	// Verify the update by fetching the case
	updatedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, createdCase.UnderscoreId).Execute()
	require.NoError(t, err)
	require.Equal(t, "Updated Case Title", updatedCase.Title)
	require.Equal(t, int32(4), updatedCase.Severity)
	require.Contains(t, updatedCase.Tags, "updated")
}

// TestManageDeleteAlert tests deleting an alert via the manage-entities tool
func TestManageDeleteAlert(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	// Create an alert to delete
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testAlert := testutils.MockInputAlert()
	testAlert.Title = "Alert to Delete"
	testAlert.SourceRef = "test-delete-alert-001"

	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdAlert)

	// Verify the alert exists
	_, _, err = hiveClient.AlertAPI.GetAlert(authContext, createdAlert.UnderscoreId).Execute()
	require.NoError(t, err)

	// Delete the alert using manage-entities
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "delete",
				"entity-type": "alert",
				"entity-ids":  []string{createdAlert.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "delete", structuredData["operation"])

	// Verify the alert no longer exists
	_, resp, err := hiveClient.AlertAPI.GetAlert(authContext, createdAlert.UnderscoreId).Execute()
	require.Error(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 404, resp.StatusCode, "Alert should return 404 after deletion")
}

// TestManageAddCommentToCase tests adding a comment to a case via the manage-entities tool
func TestManageAddCommentToCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	// Create a case to comment on
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Comment Testing"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	// Add a comment using manage-entities
	commentText := "This is a test comment added via the MCP tool. Investigation is ongoing."

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "comment",
				"entity-type": "case",
				"entity-ids":  []string{createdCase.UnderscoreId},
				"comment":     commentText,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "comment", structuredData["operation"])
	require.Equal(t, "case", structuredData["entityType"])

	// Verify the comment response contains our comment data
	resultsArray, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, resultsArray)

	// Get the first result
	firstResult, ok := resultsArray[0].(map[string]any)
	require.True(t, ok)

	// The actual comment data is in the "result" field
	commentData, ok := firstResult["result"].(map[string]any)
	require.True(t, ok)

	// Verify the comment message
	commentMessage, ok := commentData["message"].(string)
	require.True(t, ok)
	require.Equal(t, commentText, commentMessage)
}

// TestManageCreateTaskInCase tests creating a task within a case via the manage-entities tool
func TestManageCreateTaskInCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	// Create a case to add tasks to
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Task Creation"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	// Create a task in the case using manage-entities
	taskData := map[string]interface{}{
		"title":       "Investigate suspicious IP address",
		"description": "Check logs for connections to 192.168.1.100",
		"status":      "Waiting",
		"mandatory":   true,
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "create",
				"entity-type": "task",
				"entity-ids":  []string{createdCase.UnderscoreId}, // Parent case ID
				"entity-data": taskData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "create", structuredData["operation"])
	require.Equal(t, "task", structuredData["entityType"])

	resultCase, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	// Verify the task was created
	taskID, ok := resultCase["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, taskID)
	require.Equal(t, "Investigate suspicious IP address", resultCase["title"])

	// Verify the task exists in TheHive
	fetchedTask, _, err := hiveClient.TaskAPI.GetTask(authContext, taskID).Execute()
	require.NoError(t, err)
	require.Equal(t, "Investigate suspicious IP address", fetchedTask.Title)
	require.Equal(t, "Waiting", fetchedTask.Status)
	require.True(t, fetchedTask.Mandatory)
}

// TestManageCreateObservableInCase tests creating an observable within a case
func TestManageCreateObservableInCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	// Create a case to add observables to
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Observable Creation"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	// Create an observable in the case using manage-entities
	observableData := map[string]interface{}{
		"dataType": "ip",
		"data":     "192.168.1.100",
		"message":  "Suspicious IP address detected in firewall logs",
		"tlp":      2,
		"ioc":      true,
		"sighted":  true,
		"tags":     []string{"malicious", "firewall"},
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "create",
				"entity-type": "observable",
				"entity-ids":  []string{createdCase.UnderscoreId}, // Parent case ID
				"entity-data": observableData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "create", structuredData["operation"])
	require.Equal(t, "observable", structuredData["entityType"])

	resultArray, ok := structuredData["result"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, resultArray)

	resultData, ok := resultArray[0].(map[string]any)
	require.True(t, ok)

	// Verify the observable was created
	observableID, ok := resultData["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, observableID)
	require.Equal(t, "ip", resultData["dataType"])
	require.Equal(t, "192.168.1.100", resultData["data"])

	// Verify the observable exists in TheHive
	fetchedObservable, _, err := hiveClient.ObservableAPI.GetObservable(authContext, observableID).Execute()
	require.NoError(t, err)
	require.Equal(t, "ip", fetchedObservable.DataType)
	require.Equal(t, "192.168.1.100", *fetchedObservable.Data) // Only Data is a pointer
	require.True(t, fetchedObservable.Ioc)
	require.True(t, fetchedObservable.Sighted)
}

// TestManageUpdateMultipleEntities tests batch updating multiple cases
func TestManageUpdateMultipleEntities(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	// Create multiple cases to update
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	var caseIDs []string

	for i := 1; i <= 3; i++ {
		testCase := testutils.MockInputCase()
		testCase.Title = fmt.Sprintf("Case %d for Batch Update", i)
		severity := int32(2)
		testCase.Severity = &severity

		createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
		require.NoError(t, err)
		caseIDs = append(caseIDs, createdCase.UnderscoreId)
	}

	// Update all cases with the same data
	updateData := map[string]interface{}{
		"severity": 4,
		"tags":     []string{"batch-updated", "urgent"},
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "update",
				"entity-type": "case",
				"entity-ids":  caseIDs,
				"entity-data": updateData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "update", structuredData["operation"])

	// Verify all cases were updated
	for _, caseID := range caseIDs {
		updatedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, caseID).Execute()
		require.NoError(t, err)
		require.Equal(t, int32(4), updatedCase.Severity)
		require.Contains(t, updatedCase.Tags, "batch-updated")
		require.Contains(t, updatedCase.Tags, "urgent")
	}
}

// TestManageWithAnalystPermissions tests analyst permissions allow create/update/comment but deny delete
func TestManageWithAnalystPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, "../../../docs/examples/permissions/analyst.yaml")

	// Test 1: Create alert should succeed with analyst permissions
	alertData := map[string]interface{}{
		"type":        "test-type",
		"source":      "test-source",
		"sourceRef":   "test-analyst-create-001",
		"title":       "Analyst Test Alert",
		"description": "Testing analyst permissions",
		"severity":    2,
		"tlp":         2,
		"pap":         2,
		"tags":        []string{"analyst-test"},
	}

	createRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "create",
				"entity-type": "alert",
				"entity-data": alertData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Create should succeed with analyst permissions")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	resultsAlert, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)
	alertID := resultsAlert["_id"].(string)
	require.NotEmpty(t, alertID)

	// Test 2: Delete alert should fail with analyst permissions
	deleteRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "delete",
				"entity-type": "alert",
				"entity-ids":  []string{alertID},
			},
		},
	}

	result, err = mcpClient.CallTool(t.Context(), deleteRequest)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError, "Delete should be denied with analyst permissions")
	require.Contains(t, result.Content[0].(mcp.TextContent).Text, "not permitted")

	// Verify alert still exists
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	fetchedAlert, _, err := hiveClient.AlertAPI.GetAlert(authContext, alertID).Execute()
	require.NoError(t, err)
	require.Equal(t, alertID, fetchedAlert.UnderscoreId)
}

// TestManageWithReadOnlyPermissions tests read-only permissions deny all manage operations
func TestManageWithReadOnlyPermissions(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, "")

	// Test 1: Create alert should fail with read-only permissions
	alertData := map[string]interface{}{
		"type":        "test-type",
		"source":      "test-source",
		"sourceRef":   "test-readonly-create-001",
		"title":       "ReadOnly Test Alert",
		"description": "Testing read-only permissions",
		"severity":    2,
		"tlp":         2,
		"pap":         2,
	}

	createRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "create",
				"entity-type": "alert",
				"entity-data": alertData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError, "Create should be denied with read-only permissions")
	require.Contains(t, result.Content[0].(mcp.TextContent).Text, "not permitted")

	// Test 2: Comment should also fail with read-only permissions
	commentRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "manage-entities",
			Arguments: map[string]any{
				"operation":   "comment",
				"entity-type": "case",
				"entity-ids":  []string{"~123"},
				"comment":     "Test comment",
			},
		},
	}

	result, err = mcpClient.CallTool(t.Context(), commentRequest)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError, "Comment should be denied with read-only permissions")
	require.Contains(t, result.Content[0].(mcp.TextContent).Text, "not permitted")
}
