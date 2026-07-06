package manage_test

import (
	"fmt"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestManageCreateAlert(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	alertData := map[string]any{
		testFieldTypeKey:     testFieldType,
		testFieldSource:      testValueSource,
		testFieldSourceRef:   "test-create-alert-001",
		testFieldTitle:       "Test Alert via MCP",
		testFieldDescription: "This alert was created through the manage-entities tool",
		testFieldSeverity:    3,
		testFieldTLP:         2,
		testFieldPAP:         2,
		testFieldTags:        []string{"test", "automated"},
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityData: alertData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpCreate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeAlert, structuredData["entityType"])

	resultsAlert, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	// Verify the alert was created with correct data
	alertID, ok := resultsAlert["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, alertID)
	require.Equal(t, "[UNTRUSTED_DATA]Test Alert via MCP[/UNTRUSTED_DATA]", resultsAlert[testFieldTitle])
	severityValue, ok := resultsAlert[testFieldSeverity].(float64)
	require.True(t, ok)
	require.InDelta(t, float64(3), severityValue, 0.0001)

	// Verify the alert exists in TheHive by fetching it
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	fetchedAlert, _, err := hiveClient.AlertAPI.GetAlert(authContext, alertID).Execute()
	require.NoError(t, err)
	require.Equal(t, "Test Alert via MCP", fetchedAlert.Title)
}

func TestManageUpdateCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Original Case Title"
	severity := int32(2)
	testCase.Severity = &severity
	testCase.Tags = []string{"initial"}

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	updateData := map[string]any{
		testFieldTitle:       "Updated Case Title",
		testFieldSeverity:    4,
		testFieldDescription: "Updated description through MCP tool",
		testFieldTags:        []string{"initial", "updated"},
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpUpdate,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{createdCase.UnderscoreId},
				testArgEntityData: updateData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpUpdate, structuredData[testArgOperation])

	updatedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, createdCase.UnderscoreId).Execute()
	require.NoError(t, err)
	require.Equal(t, "Updated Case Title", updatedCase.Title)
	require.Equal(t, int32(4), updatedCase.Severity)
	require.Contains(t, updatedCase.Tags, "updated")
}

func TestManageDeleteAlert(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testAlert := testutils.MockInputAlert()
	testAlert.Title = "Alert to Delete"
	testAlert.SourceRef = "test-delete-alert-001"

	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdAlert)

	_, _, err = hiveClient.AlertAPI.GetAlert(authContext, createdAlert.UnderscoreId).Execute()
	require.NoError(t, err)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpDelete,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityIDs:  []string{createdAlert.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpDelete, structuredData[testArgOperation])

	_, resp, err := hiveClient.AlertAPI.GetAlert(authContext, createdAlert.UnderscoreId).Execute()
	require.Error(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 404, resp.StatusCode, "Alert should return 404 after deletion")
}

func TestManageAddCommentToCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Comment Testing"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	commentText := "This is a test comment added via the MCP tool. Investigation is ongoing."

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpComment,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{createdCase.UnderscoreId},
				testOpComment:     commentText,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpComment, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeCase, structuredData["entityType"])

	resultsArray, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, resultsArray)

	firstResult, ok := resultsArray[0].(map[string]any)
	require.True(t, ok)

	commentID, ok := firstResult["commentId"].(string)
	require.True(t, ok)

	listOp := thehive.NewInputQueryGenericOperation("listComment")
	filterOp := map[string]any{
		"_name": "filter",
		"_eq": map[string]any{
			"_field": "_id",
			"_value": commentID,
		},
	}
	query := []thehive.InputQueryNamedOperation{
		thehive.InputQueryGenericOperationAsInputQueryNamedOperation(listOp),
		thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&filterOp),
	}
	hiveQuery := thehive.InputQuery{
		Query: query,
	}
	results, _, err := hiveClient.QueryAndExportAPI.QueryAPI(authContext).InputQuery(hiveQuery).Execute()
	require.NoError(t, err)

	fetchedComments, ok := results.([]any)
	require.True(t, ok)
	require.NotEmpty(t, fetchedComments)
	fetchedComment, ok := fetchedComments[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, commentText, fetchedComment[testFieldMessage])
}

func TestManageCreateTaskInCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Task Creation"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	taskData := map[string]any{
		testFieldTitle:       "Investigate suspicious IP address",
		testFieldDescription: "Check logs for connections to 192.168.1.100",
		"status":             "Waiting",
		"mandatory":          true,
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypeTask,
				testArgEntityIDs:  []string{createdCase.UnderscoreId},
				testArgEntityData: taskData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpCreate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeTask, structuredData["entityType"])

	resultCase, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	taskID, ok := resultCase["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, taskID)
	require.Equal(t, "[UNTRUSTED_DATA]Investigate suspicious IP address[/UNTRUSTED_DATA]", resultCase[testFieldTitle])

	fetchedTask, _, err := hiveClient.TaskAPI.GetTask(authContext, taskID).Execute()
	require.NoError(t, err)
	require.Equal(t, "Investigate suspicious IP address", fetchedTask.Title)
	require.Equal(t, "Waiting", fetchedTask.Status)
	require.True(t, fetchedTask.Mandatory)
}

func TestManageCreateObservableInCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Observable Creation"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	observableData := map[string]any{
		testFieldDataType: "ip",
		testFieldData:     "192.168.1.100",
		testFieldMessage:  "Suspicious IP address detected in firewall logs",
		testFieldTLP:      2,
		"ioc":             true,
		"sighted":         true,
		testFieldTags:     []string{"malicious", "firewall"},
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypeObservable,
				testArgEntityIDs:  []string{createdCase.UnderscoreId},
				testArgEntityData: observableData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpCreate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeObservable, structuredData["entityType"])

	resultArray, ok := structuredData["result"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, resultArray)

	resultData, ok := resultArray[0].(map[string]any)
	require.True(t, ok)

	observableID, ok := resultData["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, observableID)
	require.Equal(t, "ip", resultData[testFieldDataType])

	fetchedObservable, _, err := hiveClient.ObservableAPI.GetObservable(authContext, observableID).Execute()
	require.NoError(t, err)
	require.Equal(t, "ip", fetchedObservable.DataType)
	require.Equal(t, "192.168.1.100", *fetchedObservable.Data) // Only Data is a pointer
	require.True(t, fetchedObservable.Ioc)
	require.True(t, fetchedObservable.Sighted)
}

// Regression: creating an observable in a case returned the alert-endpoint 403
// even though case creation succeeded (201). Must report IsError=false.
func TestManageCreateObservableInCaseReportsSuccess(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Observable Success Reporting"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	// Parent is a case ID, not an alert ID.
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypeObservable,
				testArgEntityIDs:  []string{createdCase.UnderscoreId},
				testArgEntityData: map[string]any{
					testFieldDataType: "ip",
					testFieldData:     "10.77.77.77",
					testFieldMessage:  "test observable",
				},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.False(t, result.IsError, "Creating an observable in a valid case must not return IsError=true")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpCreate, structuredData[testArgOperation])

	resultArray, ok := structuredData["result"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, resultArray, "Result should contain the created observable")

	obs, ok := resultArray[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ip", obs[testFieldDataType])
	require.NotEmpty(t, obs["_id"])
}

func TestManageUpdateMultipleEntities(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

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

	updateData := map[string]any{
		testFieldSeverity: 4,
		testFieldTags:     []string{"batch-updated", "urgent"},
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpUpdate,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  caseIDs,
				testArgEntityData: updateData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpUpdate, structuredData[testArgOperation])

	for _, caseID := range caseIDs {
		updatedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, caseID).Execute()
		require.NoError(t, err)
		require.Equal(t, int32(4), updatedCase.Severity)
		require.Contains(t, updatedCase.Tags, "batch-updated")
		require.Contains(t, updatedCase.Tags, "urgent")
	}
}

// Analyst permissions allow create but deny delete.
func TestManageWithAnalystPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, testutils.PermissionsFixture(t, "analyst.yaml"))

	alertData := map[string]any{
		testFieldTypeKey:     testFieldType,
		testFieldSource:      testValueSource,
		testFieldSourceRef:   "test-analyst-create-001",
		testFieldTitle:       "Analyst Test Alert",
		testFieldDescription: "Testing analyst permissions",
		testFieldSeverity:    2,
		testFieldTLP:         2,
		testFieldPAP:         2,
		testFieldTags:        []string{"analyst-test"},
	}

	createRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityData: alertData,
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

	alertID, ok := resultsAlert["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, alertID)

	deleteRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpDelete,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityIDs:  []string{alertID},
			},
		},
	}

	result, err = mcpClient.CallTool(t.Context(), deleteRequest)
	require.NoError(t, err)
	require.NotNil(t, result)
	testutils.RequirePermissionDenied(t, result)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	fetchedAlert, _, err := hiveClient.AlertAPI.GetAlert(authContext, alertID).Execute()
	require.NoError(t, err)
	require.Equal(t, alertID, fetchedAlert.UnderscoreId)
}

// Read-only permissions deny all manage operations.
func TestManageWithReadOnlyPermissions(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, "")

	alertData := map[string]any{
		testFieldTypeKey:     testFieldType,
		testFieldSource:      testValueSource,
		testFieldSourceRef:   "test-readonly-create-001",
		testFieldTitle:       "ReadOnly Test Alert",
		testFieldDescription: "Testing read-only permissions",
		testFieldSeverity:    2,
		testFieldTLP:         2,
		testFieldPAP:         2,
	}

	createRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityData: alertData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, result)
	testutils.RequirePermissionDenied(t, result)

	commentRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpComment,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{testEntityID},
				testOpComment:     "Test comment",
			},
		},
	}

	result, err = mcpClient.CallTool(t.Context(), commentRequest)
	require.NoError(t, err)
	require.NotNil(t, result)
	testutils.RequirePermissionDenied(t, result)
}

func TestManagePromoteAlert(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testAlert := testutils.MockInputAlert()
	testAlert.Title = "Alert to Promote"
	testAlert.SourceRef = "test-promote-alert-001"

	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdAlert)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpPromote,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityIDs:  []string{createdAlert.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpPromote, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeCase, structuredData["entityType"])

	caseResult, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	caseID, ok := caseResult["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, caseID)

	fetchedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, caseID).Execute()
	require.NoError(t, err)
	require.NotNil(t, fetchedCase)
}

func TestManageMergeCases(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	var caseIDs []string

	for i := 1; i <= 2; i++ {
		testCase := testutils.MockInputCase()
		testCase.Title = fmt.Sprintf("Case %d for Merging", i)

		createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
		require.NoError(t, err)

		caseIDs = append(caseIDs, createdCase.UnderscoreId)
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpMerge,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  caseIDs,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpMerge, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeCase, structuredData["entityType"])

	caseResult, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	mergedCaseID, ok := caseResult["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, mergedCaseID)

	fetchedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, mergedCaseID).Execute()
	require.NoError(t, err)
	require.NotNil(t, fetchedCase)
}

func TestManageMergeAlertsIntoCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	testCase := testutils.MockInputCase()
	testCase.Title = "Target Case for Alert Merge"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	var alertIDs []string

	for i := 1; i <= 2; i++ {
		testAlert := testutils.MockInputAlert()
		testAlert.Title = fmt.Sprintf("Alert %d to Merge", i)
		testAlert.SourceRef = fmt.Sprintf("test-merge-alert-%03d", i)

		createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
		require.NoError(t, err)

		alertIDs = append(alertIDs, createdAlert.UnderscoreId)
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpMerge,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityIDs:  alertIDs,
				testArgTargetID:   createdCase.UnderscoreId,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpMerge, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeCase, structuredData["entityType"])

	resultCase, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	targetCaseID, ok := resultCase["_id"].(string)
	require.True(t, ok)
	require.Equal(t, createdCase.UnderscoreId, targetCaseID)
}

func TestManageMergeObservables(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Observable Merge"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	// Duplicate observables so the merge has something to deduplicate.
	for i := 1; i <= 2; i++ {
		observableData := map[string]any{
			testFieldDataType: "ip",
			testFieldData:     "192.168.1.100",
			testFieldMessage:  "Duplicate IP for testing merge",
			"ioc":             true,
		}

		createRequest := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: testToolName,
				Arguments: map[string]any{
					testArgOperation:  testOpCreate,
					testArgEntityType: types.EntityTypeObservable,
					testArgEntityIDs:  []string{createdCase.UnderscoreId},
					testArgEntityData: observableData,
				},
			},
		}

		_, err := mcpClient.CallTool(t.Context(), createRequest)
		require.NoError(t, err)
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpMerge,
				testArgEntityType: types.EntityTypeObservable,
				testArgTargetID:   createdCase.UnderscoreId,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpMerge, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeObservable, structuredData["entityType"])

	targetCaseID, ok := structuredData["targetId"].(string)
	require.True(t, ok)
	require.Equal(t, createdCase.UnderscoreId, targetCaseID)
}

// Promote is allowed with analyst permissions.
func TestManagePromoteWithAnalystPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, testutils.PermissionsFixture(t, "analyst.yaml"))

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testAlert := testutils.MockInputAlert()
	testAlert.Title = "Alert for Analyst Promote Test"
	testAlert.SourceRef = "test-analyst-promote-001"

	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdAlert)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpPromote,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityIDs:  []string{createdAlert.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Promote should succeed with analyst permissions")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpPromote, structuredData[testArgOperation])
}

// Merge is allowed with analyst permissions.
func TestManageMergeWithAnalystPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, testutils.PermissionsFixture(t, "analyst.yaml"))

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	var caseIDs []string

	for i := 1; i <= 2; i++ {
		testCase := testutils.MockInputCase()
		testCase.Title = fmt.Sprintf("Case %d for Analyst Merge Test", i)

		createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
		require.NoError(t, err)

		caseIDs = append(caseIDs, createdCase.UnderscoreId)
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpMerge,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  caseIDs,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Merge should succeed with analyst permissions")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpMerge, structuredData[testArgOperation])
}

func TestManagePromoteWithReadOnlyPermissions(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, "")

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpPromote,
				testArgEntityType: types.EntityTypeAlert,
				testArgEntityIDs:  []string{testEntityID},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	testutils.RequirePermissionDenied(t, result)
}

func TestManageCreateProcedureInCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Procedure Creation"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	// Use ISO date strings — the MCP tool must handle conversion to timestamps internally
	procedureData := map[string]any{
		"patternId":          testutils.TestMITREPatternID,
		"occurDate":          "2023-11-14T22:13:20",
		"tactic":             "execution",
		testFieldDescription: "Test procedure for Command and Scripting Interpreter",
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypeProcedure,
				testArgEntityIDs:  []string{createdCase.UnderscoreId},
				testArgEntityData: procedureData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "create procedure should succeed")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpCreate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeProcedure, structuredData["entityType"])

	procedureResult, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)
	procedureID, ok := procedureResult["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, procedureID)
}

func TestManageUpdateProcedure(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Procedure Update"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	// Set up via the raw API; the MCP update is under test.
	input := thehive.NewInputProcedure(testutils.TestMITREPatternID, int64(1700000000000))
	input.SetTactic("execution")
	input.SetDescription("Original description")

	createdProcedure, _, err := hiveClient.TTPAPI.CreateProcedureForCase(authContext, createdCase.UnderscoreId).InputProcedure(*input).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdProcedure)

	// ISO date strings: the MCP converts them to timestamps.
	updateData := map[string]any{
		testFieldDescription: "Updated description via MCP",
		"occurDate":          "2023-11-15T10:00:00",
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpUpdate,
				testArgEntityType: types.EntityTypeProcedure,
				testArgEntityIDs:  []string{createdProcedure.UnderscoreId},
				testArgEntityData: updateData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "update procedure should succeed")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpUpdate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeProcedure, structuredData["entityType"])
}

func TestManageDeleteProcedure(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Procedure Deletion"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	// Set up via the raw API; the MCP delete is under test.
	input := thehive.NewInputProcedure(testutils.TestMITREPatternID, int64(1700000000000))
	input.SetTactic("execution")

	createdProcedure, _, err := hiveClient.TTPAPI.CreateProcedureForCase(authContext, createdCase.UnderscoreId).InputProcedure(*input).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdProcedure)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpDelete,
				testArgEntityType: types.EntityTypeProcedure,
				testArgEntityIDs:  []string{createdProcedure.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "delete procedure should succeed")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpDelete, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeProcedure, structuredData["entityType"])

	resp, err := hiveClient.TTPAPI.DeleteProcedure(authContext, createdProcedure.UnderscoreId).Execute()
	require.Error(t, err)
	require.Equal(t, 404, resp.StatusCode)
}

func TestManageMergeWithReadOnlyPermissions(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, "")

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpMerge,
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{testEntityID, "~456"},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	testutils.RequirePermissionDenied(t, result)
}

func TestManageCreateCaseTemplate(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	templateData := map[string]any{
		"name":               "Test-MCP-Template",
		"displayName":        "Test MCP Template",
		testFieldDescription: "A case template created via MCP for testing",
		testFieldSeverity:    2,
		testFieldTags:        []string{"test", "mcp"},
		"tasks": []map[string]any{
			{testFieldTitle: "Initial triage", testFieldDescription: "Perform initial triage of the incident"},
		},
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypeCaseTemplate,
				testArgEntityData: templateData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Case template creation should succeed")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpCreate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeCaseTemplate, structuredData["entityType"])

	resultData, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	templateID, ok := resultData["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, templateID)
	// DL-6006: name/displayName are free text, so they are wrapped.
	require.Equal(t, "[UNTRUSTED_DATA]Test-MCP-Template[/UNTRUSTED_DATA]", resultData["name"])
	require.Equal(t, "[UNTRUSTED_DATA]Test MCP Template[/UNTRUSTED_DATA]", resultData["displayName"])

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	fetchedTemplate, _, err := hiveClient.CaseTemplateAPI.GetCaseTemplate(authContext, templateID).Execute()
	require.NoError(t, err)
	require.Equal(t, "Test-MCP-Template", fetchedTemplate.Name)
}

func TestManageUpdateCaseTemplate(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	input := testutils.MockInputCaseTemplate()
	input.Name = "Update-Test-Template"

	createdTemplate, _, err := hiveClient.CaseTemplateAPI.CreateCaseTemplate(authContext).InputCreateCaseTemplate(*input).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdTemplate)

	updateData := map[string]any{
		"displayName":        "Updated Display Name",
		testFieldDescription: "Updated description via MCP",
		testFieldSeverity:    3,
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpUpdate,
				testArgEntityType: types.EntityTypeCaseTemplate,
				testArgEntityIDs:  []string{createdTemplate.UnderscoreId},
				testArgEntityData: updateData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Case template update should succeed")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpUpdate, structuredData[testArgOperation])

	fetchedTemplate, _, err := hiveClient.CaseTemplateAPI.GetCaseTemplate(authContext, createdTemplate.UnderscoreId).Execute()
	require.NoError(t, err)
	require.Equal(t, "Updated Display Name", fetchedTemplate.DisplayName)
	require.Equal(t, int32(3), *fetchedTemplate.Severity)
}

func TestManageDeleteCaseTemplate(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	input := testutils.MockInputCaseTemplate()
	input.Name = "Delete-Test-Template"

	createdTemplate, _, err := hiveClient.CaseTemplateAPI.CreateCaseTemplate(authContext).InputCreateCaseTemplate(*input).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdTemplate)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpDelete,
				testArgEntityType: types.EntityTypeCaseTemplate,
				testArgEntityIDs:  []string{createdTemplate.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Case template deletion should succeed")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpDelete, structuredData[testArgOperation])

	_, resp, err := hiveClient.CaseTemplateAPI.GetCaseTemplate(authContext, createdTemplate.UnderscoreId).Execute()
	require.Error(t, err)
	require.Equal(t, 404, resp.StatusCode)
}

func TestManageApplyTemplateToCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	input := testutils.MockInputCaseTemplate()
	input.Name = "Apply-Test-Template"
	severity := int32(3)
	input.Severity = &severity
	input.Tasks = []thehive.InputCreateTask{
		{Title: "Template task to import"},
	}

	createdTemplate, _, err := hiveClient.CaseTemplateAPI.CreateCaseTemplate(authContext).InputCreateCaseTemplate(*input).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdTemplate)

	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Template Application"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  "apply-template",
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{createdCase.UnderscoreId},
				testArgTargetID:   createdTemplate.UnderscoreId,
				testArgEntityData: map[string]any{
					"updateSeverity": true,
					"importTasks":    []string{"Template task to import"},
				},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Apply template should succeed")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "apply-template", structuredData[testArgOperation])
	require.Equal(t, createdTemplate.UnderscoreId, structuredData["templateId"])

	caseIDs, ok := structuredData["caseIds"].([]any)
	require.True(t, ok)
	require.Contains(t, caseIDs, createdCase.UnderscoreId)
}

// Apply-template is allowed with analyst permissions.
func TestManageApplyTemplateWithAnalystPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, testutils.PermissionsFixture(t, "analyst.yaml"))

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	input := testutils.MockInputCaseTemplate()
	input.Name = "Analyst-Apply-Template"

	createdTemplate, _, err := hiveClient.CaseTemplateAPI.CreateCaseTemplate(authContext).InputCreateCaseTemplate(*input).Execute()
	require.NoError(t, err)

	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Analyst Apply Template Test"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  "apply-template",
				testArgEntityType: types.EntityTypeCase,
				testArgEntityIDs:  []string{createdCase.UnderscoreId},
				testArgTargetID:   createdTemplate.UnderscoreId,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Apply template should succeed with analyst permissions")
}

// Creating case templates is denied for analysts.
func TestManageCaseTemplateCreateDeniedWithAnalystPermissions(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, testutils.PermissionsFixture(t, "analyst.yaml"))

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypeCaseTemplate,
				testArgEntityData: map[string]any{
					"name": "Analyst-Created-Template",
				},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	testutils.RequirePermissionDenied(t, result)
}

func TestManageCreatePageInCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Page Creation"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	pageData := map[string]any{
		testFieldTitle:    "Investigation Notes",
		testFieldContent:  "## Summary\nInitial findings from the investigation.",
		testFieldCategory: testCategoryDefault,
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypePage,
				testArgEntityIDs:  []string{createdCase.UnderscoreId},
				testArgEntityData: pageData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpCreate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypePage, structuredData["entityType"])

	resultData, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	pageID, ok := resultData["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, pageID)
	require.Equal(t, "[UNTRUSTED_DATA]Investigation Notes[/UNTRUSTED_DATA]", resultData[testFieldTitle])
	// DL-6006: testFieldCategory is a user-defined label, so it is wrapped.
	require.Equal(t, "[UNTRUSTED_DATA]Default[/UNTRUSTED_DATA]", resultData[testFieldCategory])
}

func TestManageCreateStandalonePage(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	pageData := map[string]any{
		testFieldTitle:    "Incident Response Runbook",
		testFieldContent:  "## Procedure\n1. Identify scope\n2. Contain threat\n3. Eradicate.",
		testFieldCategory: testCategoryDefault,
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypePage,
				testArgEntityData: pageData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpCreate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypePage, structuredData["entityType"])

	resultData, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	pageID, ok := resultData["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, pageID)
	require.Equal(t, "[UNTRUSTED_DATA]Incident Response Runbook[/UNTRUSTED_DATA]", resultData[testFieldTitle])
}

func TestManageUpdatePage(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Page Update"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	inputPage := thehive.InputCreatePage{
		Title:    "Original Page Title",
		Content:  "## Original\ncontent.",
		Category: testCategoryDefault,
	}
	createdPage, _, err := hiveClient.PageAPI.CreateAPageInACase(authContext, createdCase.UnderscoreId).InputCreatePage(inputPage).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdPage)

	updateData := map[string]any{
		testFieldTitle:   "Updated Page Title",
		testFieldContent: "## Updated\nNew content after update.",
	}

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpUpdate,
				testArgEntityType: types.EntityTypePage,
				testArgEntityIDs:  []string{createdPage.UnderscoreId},
				testArgEntityData: updateData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpUpdate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypePage, structuredData["entityType"])
}

func TestManageDeletePage(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Page Deletion"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	inputPage := thehive.InputCreatePage{
		Title:    "Page to Delete",
		Content:  "This page will be deleted.",
		Category: testCategoryDefault,
	}
	createdPage, _, err := hiveClient.PageAPI.CreateAPageInACase(authContext, createdCase.UnderscoreId).InputCreatePage(inputPage).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdPage)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpDelete,
				testArgEntityType: types.EntityTypePage,
				testArgEntityIDs:  []string{createdPage.UnderscoreId},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	require.Equal(t, testOpDelete, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypePage, structuredData["entityType"])
}

// Analyst permissions allow page create but deny delete.
func TestManagePageWithAnalystPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, testutils.PermissionsFixture(t, "analyst.yaml"))

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	pageData := map[string]any{
		testFieldTitle:    "Analyst Created Page",
		testFieldContent:  "## Content\nPage created by analyst.",
		testFieldCategory: testCategoryDefault,
	}

	createRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpCreate,
				testArgEntityType: types.EntityTypePage,
				testArgEntityData: pageData,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), createRequest)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Page creation should succeed with analyst permissions")

	testCase := testutils.MockInputCase()
	testCase.Title = "Case for Analyst Page Permission Test"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)

	inputPage := thehive.InputCreatePage{
		Title:    "Page for Analyst Delete Test",
		Content:  "Content",
		Category: testCategoryDefault,
	}
	createdPage, _, err := hiveClient.PageAPI.CreateAPageInACase(authContext, createdCase.UnderscoreId).InputCreatePage(inputPage).Execute()
	require.NoError(t, err)

	deleteRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: testToolName,
			Arguments: map[string]any{
				testArgOperation:  testOpDelete,
				testArgEntityType: types.EntityTypePage,
				testArgEntityIDs:  []string{createdPage.UnderscoreId},
			},
		},
	}

	result, err = mcpClient.CallTool(t.Context(), deleteRequest)
	require.NoError(t, err)
	require.NotNil(t, result)
	testutils.RequirePermissionDenied(t, result)
}
