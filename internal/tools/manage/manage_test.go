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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityData: alertData,
	})
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
	authContext := testutils.GetAuthContext(t)
	fetchedAlert, _, err := hiveClient.AlertAPI.GetAlert(authContext, alertID).Execute()
	require.NoError(t, err)
	require.Equal(t, "Test Alert via MCP", fetchedAlert.Title)
}

func TestManageUpdateCase(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpUpdate,
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  []string{createdCase.UnderscoreId},
		testArgEntityData: updateData,
	})
	require.Equal(t, testOpUpdate, structuredData[testArgOperation])

	updatedCase, _, err := hiveClient.CaseAPI.GetCase(authContext, createdCase.UnderscoreId).Execute()
	require.NoError(t, err)
	require.Equal(t, "Updated Case Title", updatedCase.Title)
	require.Equal(t, int32(4), updatedCase.Severity)
	require.Contains(t, updatedCase.Tags, "updated")
}

func TestManageDeleteAlert(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
	createdAlert := createAlert(t, hiveClient, "Alert to Delete", "test-delete-alert-001")

	_, _, err := hiveClient.AlertAPI.GetAlert(authContext, createdAlert.UnderscoreId).Execute()
	require.NoError(t, err)

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpDelete,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityIDs:  []string{createdAlert.UnderscoreId},
	})
	require.Equal(t, testOpDelete, structuredData[testArgOperation])

	_, resp, err := hiveClient.AlertAPI.GetAlert(authContext, createdAlert.UnderscoreId).Execute()
	require.Error(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 404, resp.StatusCode, "Alert should return 404 after deletion")
}

func TestManageAddCommentToCase(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
	createdCase := createCase(t, hiveClient, "Case for Comment Testing")

	commentText := "This is a test comment added via the MCP tool. Investigation is ongoing."

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpComment,
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  []string{createdCase.UnderscoreId},
		testOpComment:     commentText,
	})
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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
	createdCase := createCase(t, hiveClient, "Case for Task Creation")

	taskData := map[string]any{
		testFieldTitle:       "Investigate suspicious IP address",
		testFieldDescription: "Check logs for connections to 192.168.1.100",
		"status":             "Waiting",
		"mandatory":          true,
	}

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeTask,
		testArgEntityIDs:  []string{createdCase.UnderscoreId},
		testArgEntityData: taskData,
	})
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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
	createdCase := createCase(t, hiveClient, "Case for Observable Creation")

	observableData := map[string]any{
		testFieldDataType: "ip",
		testFieldData:     "192.168.1.100",
		testFieldMessage:  "Suspicious IP address detected in firewall logs",
		testFieldTLP:      2,
		"ioc":             true,
		"sighted":         true,
		testFieldTags:     []string{"malicious", "firewall"},
	}

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeObservable,
		testArgEntityIDs:  []string{createdCase.UnderscoreId},
		testArgEntityData: observableData,
	})
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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	createdCase := createCase(t, hiveClient, "Case for Observable Success Reporting")

	// Parent is a case ID, not an alert ID.
	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeObservable,
		testArgEntityIDs:  []string{createdCase.UnderscoreId},
		testArgEntityData: map[string]any{
			testFieldDataType: "ip",
			testFieldData:     "10.77.77.77",
			testFieldMessage:  "test observable",
		},
	})
	require.Equal(t, testOpCreate, structuredData[testArgOperation])

	resultArray, ok := structuredData["result"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, resultArray, "Result should contain the created observable")

	obs, ok := resultArray[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ip", obs[testFieldDataType])
	require.NotEmpty(t, obs["_id"])
}

// Regression: an observable without `data` (the value sent as `value`, or
// missing) made TheHive answer 2xx with an empty list, and the tool reported
// "Observable created successfully" for an observable that never existed.
func TestManageCreateObservableWithoutDataIsAnError(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	alert := createAlert(t, hiveClient, "Alert for an observable without data", "obs-without-data")

	result := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeObservable,
		testArgEntityIDs:  []string{alert.UnderscoreId},
		testArgEntityData: map[string]any{testFieldDataType: "ip", "value": "203.0.113.9"},
	})
	require.True(t, result.IsError, "a create that created nothing must be reported as an error")
}

// Regression: when one value already existed, TheHive answered 207 with the
// created and the refused values, and the tool reported a failure although it
// had created the others. A create whose values all exist already is an error.
func TestManageCreateObservableWithAnExistingValue(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	alert := createAlert(t, hiveClient, "Alert for a partial observable create", "obs-partial-create")
	create := func(values ...string) *mcp.CallToolResult {
		return testutils.CallTool(t, mcpClient, testToolName, map[string]any{
			testArgOperation:  testOpCreate,
			testArgEntityType: types.EntityTypeObservable,
			testArgEntityIDs:  []string{alert.UnderscoreId},
			testArgEntityData: map[string]any{testFieldDataType: "ip", testFieldData: values},
		})
	}

	require.False(t, create("198.51.100.7").IsError)

	partial := create("198.51.100.7", "198.51.100.8")
	require.False(t, partial.IsError, "a create that made one of two observables is a success")

	structured := testutils.StructuredData(t, partial)
	created, ok := structured["result"].([]any)
	require.True(t, ok)
	require.Len(t, created, 1)

	failures, ok := structured["failures"].([]any)
	require.True(t, ok)
	require.Len(t, failures, 1)

	failure, ok := failures[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "[UNTRUSTED_DATA]CreateError[/UNTRUSTED_DATA]", failure["type"], "TheHive's reason reaches the model, fenced")

	refused := create("198.51.100.7")
	require.True(t, refused.IsError, "a create that made nothing is an error")
	require.NotEmpty(t, refused.Content)
	text, ok := refused.Content[0].(mcp.TextContent)
	require.True(t, ok)
	require.Contains(t, text.Text, "(CreateError)", "the error names TheHive's reason")
}

func TestManageUpdateMultipleEntities(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)

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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpUpdate,
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  caseIDs,
		testArgEntityData: updateData,
	})
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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, testutils.PermissionsFixture(t, "analyst.yaml"))

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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityData: alertData,
	})

	resultsAlert, ok := structuredData["result"].(map[string]any)
	require.True(t, ok)

	alertID, ok := resultsAlert["_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, alertID)

	result := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpDelete,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityIDs:  []string{alertID},
	})
	testutils.RequirePermissionDenied(t, result)

	authContext := testutils.GetAuthContext(t)
	fetchedAlert, _, err := hiveClient.AlertAPI.GetAlert(authContext, alertID).Execute()
	require.NoError(t, err)
	require.Equal(t, alertID, fetchedAlert.UnderscoreId)
}

// Read-only permissions deny all manage operations.
func TestManageWithReadOnlyPermissions(t *testing.T) {
	testutils.Parallel(t)
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, "")

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

	result := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityData: alertData,
	})
	testutils.RequirePermissionDenied(t, result)

	result = testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpComment,
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  []string{testEntityID},
		testOpComment:     "Test comment",
	})
	testutils.RequirePermissionDenied(t, result)
}
