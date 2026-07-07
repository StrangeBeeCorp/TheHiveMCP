package manage_test

import (
	"fmt"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
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
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
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
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
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
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
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
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
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
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

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

func TestManagePromoteAlert(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	createdAlert := createAlert(t, hiveClient, "Alert to Promote", "test-promote-alert-001")

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpPromote,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityIDs:  []string{createdAlert.UnderscoreId},
	})
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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpMerge,
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  caseIDs,
	})
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

	createdCase := createCase(t, hiveClient, "Target Case for Alert Merge")

	var alertIDs []string

	for i := 1; i <= 2; i++ {
		createdAlert := createAlert(t, hiveClient, fmt.Sprintf("Alert %d to Merge", i), fmt.Sprintf("test-merge-alert-%03d", i))
		alertIDs = append(alertIDs, createdAlert.UnderscoreId)
	}

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpMerge,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityIDs:  alertIDs,
		testArgTargetID:   createdCase.UnderscoreId,
	})
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

	createdCase := createCase(t, hiveClient, "Case for Observable Merge")

	// Duplicate observables so the merge has something to deduplicate.
	for i := 1; i <= 2; i++ {
		observableData := map[string]any{
			testFieldDataType: "ip",
			testFieldData:     "192.168.1.100",
			testFieldMessage:  "Duplicate IP for testing merge",
			"ioc":             true,
		}

		testutils.CallTool(t, mcpClient, testToolName, map[string]any{
			testArgOperation:  testOpCreate,
			testArgEntityType: types.EntityTypeObservable,
			testArgEntityIDs:  []string{createdCase.UnderscoreId},
			testArgEntityData: observableData,
		})
	}

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpMerge,
		testArgEntityType: types.EntityTypeObservable,
		testArgTargetID:   createdCase.UnderscoreId,
	})
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

	createdAlert := createAlert(t, hiveClient, "Alert for Analyst Promote Test", "test-analyst-promote-001")

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpPromote,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityIDs:  []string{createdAlert.UnderscoreId},
	})
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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpMerge,
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  caseIDs,
	})
	require.Equal(t, testOpMerge, structuredData[testArgOperation])
}

func TestManagePromoteWithReadOnlyPermissions(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, "")

	result := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpPromote,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityIDs:  []string{testEntityID},
	})
	testutils.RequirePermissionDenied(t, result)
}

func TestManageCreateProcedureInCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	createdCase := createCase(t, hiveClient, "Case for Procedure Creation")

	// Use ISO date strings — the MCP tool must handle conversion to timestamps internally
	procedureData := map[string]any{
		"patternId":          testutils.TestMITREPatternID,
		"occurDate":          "2023-11-14T22:13:20",
		"tactic":             "execution",
		testFieldDescription: "Test procedure for Command and Scripting Interpreter",
	}

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeProcedure,
		testArgEntityIDs:  []string{createdCase.UnderscoreId},
		testArgEntityData: procedureData,
	})
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
	createdCase := createCase(t, hiveClient, "Case for Procedure Update")

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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpUpdate,
		testArgEntityType: types.EntityTypeProcedure,
		testArgEntityIDs:  []string{createdProcedure.UnderscoreId},
		testArgEntityData: updateData,
	})
	require.Equal(t, testOpUpdate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeProcedure, structuredData["entityType"])
}

func TestManageDeleteProcedure(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	createdCase := createCase(t, hiveClient, "Case for Procedure Deletion")

	// Set up via the raw API; the MCP delete is under test.
	input := thehive.NewInputProcedure(testutils.TestMITREPatternID, int64(1700000000000))
	input.SetTactic("execution")

	createdProcedure, _, err := hiveClient.TTPAPI.CreateProcedureForCase(authContext, createdCase.UnderscoreId).InputProcedure(*input).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdProcedure)

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpDelete,
		testArgEntityType: types.EntityTypeProcedure,
		testArgEntityIDs:  []string{createdProcedure.UnderscoreId},
	})
	require.Equal(t, testOpDelete, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypeProcedure, structuredData["entityType"])

	resp, err := hiveClient.TTPAPI.DeleteProcedure(authContext, createdProcedure.UnderscoreId).Execute()
	require.Error(t, err)
	require.Equal(t, 404, resp.StatusCode)
}

func TestManageMergeWithReadOnlyPermissions(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, "")

	result := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpMerge,
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  []string{testEntityID, "~456"},
	})
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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeCaseTemplate,
		testArgEntityData: templateData,
	})
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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpUpdate,
		testArgEntityType: types.EntityTypeCaseTemplate,
		testArgEntityIDs:  []string{createdTemplate.UnderscoreId},
		testArgEntityData: updateData,
	})
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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpDelete,
		testArgEntityType: types.EntityTypeCaseTemplate,
		testArgEntityIDs:  []string{createdTemplate.UnderscoreId},
	})
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

	createdCase := createCase(t, hiveClient, "Case for Template Application")

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  "apply-template",
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  []string{createdCase.UnderscoreId},
		testArgTargetID:   createdTemplate.UnderscoreId,
		testArgEntityData: map[string]any{
			"updateSeverity": true,
			"importTasks":    []string{"Template task to import"},
		},
	})
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

	createdCase := createCase(t, hiveClient, "Case for Analyst Apply Template Test")

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  "apply-template",
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  []string{createdCase.UnderscoreId},
		testArgTargetID:   createdTemplate.UnderscoreId,
	})
	require.Equal(t, "apply-template", structuredData[testArgOperation])
}

// Creating case templates is denied for analysts.
func TestManageCaseTemplateCreateDeniedWithAnalystPermissions(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, nil, testutils.DummyElicitationAccept, testutils.PermissionsFixture(t, "analyst.yaml"))

	result := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeCaseTemplate,
		testArgEntityData: map[string]any{
			"name": "Analyst-Created-Template",
		},
	})
	testutils.RequirePermissionDenied(t, result)
}

func TestManageCreatePageInCase(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	createdCase := createCase(t, hiveClient, "Case for Page Creation")

	pageData := map[string]any{
		testFieldTitle:    "Investigation Notes",
		testFieldContent:  "## Summary\nInitial findings from the investigation.",
		testFieldCategory: testCategoryDefault,
	}

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypePage,
		testArgEntityIDs:  []string{createdCase.UnderscoreId},
		testArgEntityData: pageData,
	})
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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypePage,
		testArgEntityData: pageData,
	})
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
	createdCase := createCase(t, hiveClient, "Case for Page Update")

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

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpUpdate,
		testArgEntityType: types.EntityTypePage,
		testArgEntityIDs:  []string{createdPage.UnderscoreId},
		testArgEntityData: updateData,
	})
	require.Equal(t, testOpUpdate, structuredData[testArgOperation])
	require.Equal(t, types.EntityTypePage, structuredData["entityType"])
}

func TestManageDeletePage(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	createdCase := createCase(t, hiveClient, "Case for Page Deletion")

	inputPage := thehive.InputCreatePage{
		Title:    "Page to Delete",
		Content:  "This page will be deleted.",
		Category: testCategoryDefault,
	}
	createdPage, _, err := hiveClient.PageAPI.CreateAPageInACase(authContext, createdCase.UnderscoreId).InputCreatePage(inputPage).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdPage)

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpDelete,
		testArgEntityType: types.EntityTypePage,
		testArgEntityIDs:  []string{createdPage.UnderscoreId},
	})
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

	result := testutils.CallToolOK(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypePage,
		testArgEntityData: pageData,
	})
	require.False(t, result.IsError, "Page creation should succeed with analyst permissions")

	createdCase := createCase(t, hiveClient, "Case for Analyst Page Permission Test")

	inputPage := thehive.InputCreatePage{
		Title:    "Page for Analyst Delete Test",
		Content:  "Content",
		Category: testCategoryDefault,
	}
	createdPage, _, err := hiveClient.PageAPI.CreateAPageInACase(authContext, createdCase.UnderscoreId).InputCreatePage(inputPage).Execute()
	require.NoError(t, err)

	deleteResult := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpDelete,
		testArgEntityType: types.EntityTypePage,
		testArgEntityIDs:  []string{createdPage.UnderscoreId},
	})
	testutils.RequirePermissionDenied(t, deleteResult)
}
