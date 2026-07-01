package search_test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// Fails the test if invoked: asserts search-entities never uses the sampling/LLM path.
func unusedSamplingHandler(t *testing.T) func(context.Context, mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	return func(context.Context, mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
		t.Error("search-entities must not call the sampling/LLM path")
		return nil, fmt.Errorf("unexpected sampling call")
	}
}

func createTestAlert(t *testing.T, hiveClient *thehive.APIClient, title string, severity int32, tags []string) map[string]interface{} {
	testAlert := testutils.MockInputAlert()
	testAlert.Title = title
	testAlert.Severity = &severity
	testAlert.Tags = tags
	testAlert.SourceRef = "test-" + title

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdAlert)

	return map[string]interface{}{
		"_id":      createdAlert.UnderscoreId,
		"title":    createdAlert.Title,
		"severity": createdAlert.Severity,
	}
}

func createTestCase(t *testing.T, hiveClient *thehive.APIClient, title string, severity int32, status string, assignee string) map[string]interface{} {
	testCase := testutils.MockInputCase()
	testCase.Title = title
	testCase.Severity = &severity
	testCase.Status = &status
	if assignee != "" {
		testCase.Assignee = &assignee
	} else {
		testCase.Assignee = nil // Explicitly set to nil to remove the default assignee
	}

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	createdCase, resp, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	slog.Info("Create case response", "response", resp)
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	return map[string]interface{}{
		"_id":    createdCase.UnderscoreId,
		"title":  createdCase.Title,
		"status": createdCase.Status,
	}
}

func createTestCaseWithTaskAndAlert(t *testing.T, hiveClient *thehive.APIClient) map[string]interface{} {
	testCase := testutils.MockInputCase()
	testCase.Title = "Test case with tasks"
	testAlert := testutils.MockInputAlert()
	testAlert.Title = "Test alert 1"
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	createdCase, resp, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	slog.Info("Create case response", "response", resp)
	require.NoError(t, err)
	require.NotNil(t, createdCase)
	createdAlert, resp, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
	slog.Info("Create alert response", "response", resp)
	require.NoError(t, err)
	require.NotNil(t, createdAlert)
	_, resp, err = hiveClient.AlertAPI.MergeAlertWithCase(authContext, createdAlert.UnderscoreId, createdCase.UnderscoreId).Execute()
	require.NoError(t, err)
	require.NotNil(t, resp)

	return map[string]interface{}{
		"case_id":  createdCase.UnderscoreId,
		"alert_id": createdAlert.UnderscoreId,
	}
}

func TestSearchCasesBySeverityAndStatus(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestCase(t, hiveClient, "High severity open case", 3, "New", "")
	createTestCase(t, hiveClient, "Low severity open case", 1, "New", "")
	createTestCase(t, hiveClient, "High severity in progress case", 3, "InProgress", "")

	mcpClient := newSearchClient(t)

	casesData := searchRows(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeCase,
		"filters": map[string]any{
			"_and": []any{
				map[string]any{
					"_gte": map[string]any{
						"_field": "severity",
						"_value": 3,
					},
				},
				map[string]any{
					"_eq": map[string]any{
						"_field": "status",
						"_value": "New",
					},
				},
			},
		},
		"extra-columns": []string{"_id", "title", "severity", "status"},
	})
	require.Len(t, casesData, 1)

	caseData, ok := casesData[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "[UNTRUSTED_DATA]High severity open case[/UNTRUSTED_DATA]", caseData["title"])
	require.Equal(t, float64(3), caseData["severity"])
}

func TestSearchAlertsWithDateRange(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Recent alert", 2, []string{"recent"})
	time.Sleep(100 * time.Millisecond) // distinct timestamps
	createTestAlert(t, hiveClient, "Another recent alert", 2, []string{"recent"})

	now := time.Now()
	fromTime := now.Add(-1 * time.Hour).UnixMilli()
	toTime := now.UnixMilli()

	mcpClient := newSearchClient(t)

	alertsData := searchRows(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeAlert,
		"filters": map[string]any{
			"_between": map[string]any{
				"_field": "_createdAt",
				"_from":  fromTime,
				"_to":    toTime,
			},
		},
		"extra-columns": []string{"_id", "title", "_createdAt"},
	})
	require.GreaterOrEqual(t, len(alertsData), 2)
}

func TestSearchAlertsWithMultipleTags(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Phishing alert", 3, []string{"phishing", "email"})
	createTestAlert(t, hiveClient, "Malware alert", 3, []string{"malware", "endpoint"})
	createTestAlert(t, hiveClient, "Network alert", 2, []string{"network", "firewall"})

	mcpClient := newSearchClient(t)

	alertsData := searchRows(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeAlert,
		"filters": map[string]any{
			"_or": []any{
				map[string]any{
					"_in": map[string]any{
						"_field":  "tags",
						"_values": []any{"phishing", "malware"},
					},
				},
			},
		},
		"extra-columns": []string{"_id", "title", "tags", "severity"},
		"sort-by":       "severity",
	})
	require.Len(t, alertsData, 2)
}

func TestSearchCasesWithAssigneeAndSorting(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Assign to admin: test users don't exist in the fixture.
	createTestCase(t, hiveClient, "Admin's case 1", 2, "InProgress", "admin@thehive.local")
	time.Sleep(50 * time.Millisecond)
	createTestCase(t, hiveClient, "Admin's case 2", 3, "InProgress", "admin@thehive.local")
	// TheHive assigns the creator as assignee even when set nil, so all cases show admin.

	mcpClient := newSearchClient(t)

	casesData := searchRows(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeCase,
		"filters": map[string]any{
			"_and": []any{
				map[string]any{
					"_eq": map[string]any{
						"_field": "assignee",
						"_value": "admin@thehive.local",
					},
				},
				map[string]any{
					"_eq": map[string]any{
						"_field": "status",
						"_value": "InProgress",
					},
				},
			},
		},
		"extra-columns": []string{"_id", "title", "assignee", "_createdAt"},
		"sort-order":    "asc",
	})
	require.Len(t, casesData, 2)

	firstCase := casesData[0].(map[string]any)
	secondCase := casesData[1].(map[string]any)
	require.Equal(t, "[UNTRUSTED_DATA]Admin's case 1[/UNTRUSTED_DATA]", firstCase["title"])
	require.Equal(t, "[UNTRUSTED_DATA]Admin's case 2[/UNTRUSTED_DATA]", secondCase["title"])
}

func TestSearchAlertsWithComplexOrConditions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Critical alert", 4, []string{"critical"})
	createTestAlert(t, hiveClient, "High alert", 3, []string{"high"})
	createTestAlert(t, hiveClient, "Medium alert", 2, []string{"medium"})
	createTestAlert(t, hiveClient, "Low alert", 1, []string{"low"})

	mcpClient := newSearchClient(t)

	alertsData := searchRows(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeAlert,
		"filters": map[string]any{
			"_or": []any{
				map[string]any{
					"_eq": map[string]any{
						"_field": "severity",
						"_value": 4,
					},
				},
				map[string]any{
					"_eq": map[string]any{
						"_field": "severity",
						"_value": 3,
					},
				},
			},
		},
		"extra-columns": []string{"_id", "title", "severity"},
		"sort-by":       "severity",
	})
	require.Len(t, alertsData, 2)

	for _, alertAny := range alertsData {
		alert := alertAny.(map[string]any)
		severity := int(alert["severity"].(float64))
		require.GreaterOrEqual(t, severity, 3, "Only high (3) and critical (4) severity alerts should be returned")
	}
}

func TestSearchTasksWithLimit(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Test case for tasks"
	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)

	for i := 1; i <= 5; i++ {
		testTask := testutils.MockInputTask()
		testTask.Title = fmt.Sprintf("Task %d", i)
		_, _, err := hiveClient.TaskAPI.CreateTaskInCase(authContext, createdCase.UnderscoreId).
			InputCreateTask(*testTask).Execute()
		require.NoError(t, err)
	}

	mcpClient := newSearchClient(t)

	tasksData := searchRows(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeTask,
		"limit":       3,
	})
	require.Len(t, tasksData, 3, "Should return exactly 3 tasks as per limit")
}

func TestExtraColumnsLimitColumns(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Test alert for column override", 2, []string{"test"})

	mcpClient := newSearchClient(t)

	alertsData := searchRows(t, mcpClient, map[string]any{
		"entity-type":   types.EntityTypeAlert,
		"extra-columns": []string{"_id", "title"},
	})
	require.GreaterOrEqual(t, len(alertsData), 1)

	alertData := alertsData[0].(map[string]any)

	require.Contains(t, alertData, "_id")
	require.Contains(t, alertData, "title")

	// Filtered out despite being requested in extra-columns: not in kept_columns.
	require.NotContains(t, alertData, "severity", "severity should not be present as it's not in kept_columns")
	require.NotContains(t, alertData, "tags", "tags should not be present as it's not in kept_columns")
	require.NotContains(t, alertData, "_createdAt", "_createdAt should not be present as it's not in kept_columns")

	require.Len(t, alertData, 2, "Should only have 2 columns as specified in kept_columns")
}

func TestSearchWithAnalystPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	// Alert 1: TLP=2, PAP=2 (visible)
	alert1 := testutils.MockInputAlert()
	tlp1 := int32(2)
	pap1 := int32(2)
	alert1.Tlp = &tlp1
	alert1.Pap = &pap1
	alert1.Title = "Alert TLP2 PAP2"
	alert1.SourceRef = "test-analyst-search-001"
	createdAlert1, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*alert1).Execute()
	require.NoError(t, err)

	// Alert 2: TLP=3, PAP=1 (hidden: TLP too high)
	alert2 := testutils.MockInputAlert()
	tlp2 := int32(3)
	pap2 := int32(1)
	alert2.Tlp = &tlp2
	alert2.Pap = &pap2
	alert2.Title = "Alert TLP3 PAP1"
	alert2.SourceRef = "test-analyst-search-002"
	createdAlert2, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*alert2).Execute()
	require.NoError(t, err)

	// Alert 3: TLP=1, PAP=3 (hidden: PAP too high)
	alert3 := testutils.MockInputAlert()
	tlp3 := int32(1)
	pap3 := int32(3)
	alert3.Tlp = &tlp3
	alert3.Pap = &pap3
	alert3.Title = "Alert TLP1 PAP3"
	alert3.SourceRef = "test-analyst-search-003"
	createdAlert3, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*alert3).Execute()
	require.NoError(t, err)

	mcpClient := testutils.GetMCPTestClientWithPermissions(t, unusedSamplingHandler(t), testutils.DummyElicitationAccept, testutils.PermissionsFixture(t, "analyst.yaml"))

	alertsData := searchRows(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeAlert,
	})

	visibleIDs := make(map[string]bool)
	for _, alertInterface := range alertsData {
		alert := alertInterface.(map[string]any)
		visibleIDs[alert["_id"].(string)] = true
	}

	require.True(t, visibleIDs[createdAlert1.UnderscoreId], "Alert with TLP=2, PAP=2 should be visible")
	require.False(t, visibleIDs[createdAlert2.UnderscoreId], "Alert with TLP=3 should NOT be visible")
	require.False(t, visibleIDs[createdAlert3.UnderscoreId], "Alert with PAP=3 should NOT be visible")
}

func TestSearchWithReadOnlyPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	alert := testutils.MockInputAlert()
	alert.Title = "ReadOnly Search Test Alert"
	alert.SourceRef = "test-readonly-search-001"
	severity := int32(2)
	alert.Severity = &severity
	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*alert).Execute()
	require.NoError(t, err)

	mcpClient := testutils.GetMCPTestClientWithPermissions(t, unusedSamplingHandler(t), testutils.DummyElicitationAccept, "")

	result := callSearch(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeAlert,
	})
	require.False(t, result.IsError, "Search should succeed with read-only permissions")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	alertsData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(alertsData), 1, "Should find at least one alert")

	found := false
	for _, alertInterface := range alertsData {
		alert := alertInterface.(map[string]any)
		if alert["_id"].(string) == createdAlert.UnderscoreId {
			found = true
			break
		}
	}
	require.True(t, found, "Should find our test alert")
}

func TestSearchCasesWithCountOnly(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestCase(t, hiveClient, "High severity case 1", 3, "New", "")
	createTestCase(t, hiveClient, "High severity case 2", 3, "InProgress", "")
	createTestCase(t, hiveClient, "Low severity case", 1, "New", "")

	mcpClient := newSearchClient(t)

	structuredData := searchStructured(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeCase,
		"filters": map[string]any{
			"_gte": map[string]any{
				"_field": "severity",
				"_value": 3,
			},
		},
		"count": true,
	})

	countOnly, ok := structuredData["countOnly"].(bool)
	require.True(t, ok)
	require.True(t, countOnly)

	count, ok := structuredData["count"].(float64)
	require.True(t, ok)
	require.Equal(t, float64(2), count)

	require.Equal(t, types.EntityTypeCase, structuredData["entityType"])
	require.NotNil(t, structuredData["rawFilters"])
}

func TestSearchAlertsWithCountOnly(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Critical Alert 1", 4, []string{"malware", "phishing"})
	createTestAlert(t, hiveClient, "Critical Alert 2", 4, []string{"malware"})
	createTestAlert(t, hiveClient, "Medium Alert", 2, []string{"suspicious"})

	mcpClient := newSearchClient(t)

	structuredData := searchStructured(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeAlert,
		"filters": map[string]any{
			"_eq": map[string]any{
				"_field": "severity",
				"_value": 4,
			},
		},
		"count": true,
	})

	require.True(t, structuredData["countOnly"].(bool))
	require.Equal(t, float64(2), structuredData["count"].(float64))
	require.Equal(t, types.EntityTypeAlert, structuredData["entityType"])
}

func TestSearchCountVsRegularSearch(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestCase(t, hiveClient, "Test case 1", 2, "New", "")
	createTestCase(t, hiveClient, "Test case 2", 2, "InProgress", "")
	createTestCase(t, hiveClient, "Test case 3", 2, "New", "")

	mcpClient := newSearchClient(t)

	// Same filter, toggling only count, so the count must equal the row count.
	args := func(count bool) map[string]any {
		return map[string]any{
			"entity-type": types.EntityTypeCase,
			"filters":     map[string]any{"_eq": map[string]any{"_field": "severity", "_value": 2}},
			"count":       count,
		}
	}

	regularResults := searchRows(t, mcpClient, args(false))
	regularCount := len(regularResults)

	countData := searchStructured(t, mcpClient, args(true))

	countOnlyValue, ok := countData["count"].(float64)
	require.True(t, ok)

	require.Equal(t, float64(regularCount), countOnlyValue)
	require.Equal(t, 3, regularCount) // We created 3 test cases
}

func TestSearchExtraDataAndAdditionalQueries(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	creationResult := createTestCaseWithTaskAndAlert(t, hiveClient)

	mcpClient := newSearchClient(t)

	structuredData := searchStructured(t, mcpClient, map[string]any{
		"entity-type":        types.EntityTypeCase,
		"extra-columns":      []string{"_id", "title"},
		"extra-data":         []string{"alerts"},
		"additional-queries": []string{"tasks"},
	})

	casesData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.Len(t, casesData, 1)

	caseData := casesData[0].(map[string]any)
	require.Equal(t, creationResult["case_id"], caseData["_id"])
	require.Equal(t, "[UNTRUSTED_DATA]Test case with tasks[/UNTRUSTED_DATA]", caseData["title"])

	restults, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.Len(t, restults, 1)

	firstResult := restults[0].(map[string]any)
	extraData, ok := firstResult["extraData"].(map[string]any)
	require.True(t, ok)

	alertsData, ok := extraData["alerts"].([]any)
	require.True(t, ok)
	require.Len(t, alertsData, 1)

	alert := alertsData[0].(map[string]any)
	// DL-6006: "type" is an open ingestion-controlled label, so it is wrapped.
	require.Equal(t, "[UNTRUSTED_DATA]test[/UNTRUSTED_DATA]", alert["type"])
	require.Equal(t, "[UNTRUSTED_DATA]test[/UNTRUSTED_DATA]", alert["source"])

	tasks, ok := firstResult["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, tasks, 1)

	task := tasks[0].(map[string]any)
	require.Equal(t, "[UNTRUSTED_DATA]Test Task[/UNTRUSTED_DATA]", task["title"])
}

func createTestCaseWithComment(t *testing.T, hiveClient *thehive.APIClient) map[string]interface{} {
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Test case for comment"
	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)

	commentInput := thehive.InputComment{
		Message: "This is a test comment",
	}
	_, _, err = hiveClient.CommentAPI.CreateCommentInCase(authContext, createdCase.UnderscoreId).InputComment(commentInput).Execute()
	require.NoError(t, err)

	return map[string]interface{}{
		"case_id": createdCase.UnderscoreId,
	}
}

func TestSearchAdditionalQueriesComments(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	creationResult := createTestCaseWithComment(t, hiveClient)

	mcpClient := newSearchClient(t)

	casesData := searchRows(t, mcpClient, map[string]any{
		"entity-type":        types.EntityTypeCase,
		"extra-columns":      []string{"_id", "title"},
		"additional-queries": []string{"comments"},
	})
	require.Len(t, casesData, 1)

	caseData := casesData[0].(map[string]any)
	require.Equal(t, creationResult["case_id"], caseData["_id"])
	require.Equal(t, "[UNTRUSTED_DATA]Test case for comment[/UNTRUSTED_DATA]", caseData["title"])

	comments, ok := caseData["comments"].([]any)
	require.True(t, ok)
	require.Len(t, comments, 1)

	comment := comments[0].(map[string]any)
	require.Equal(t, "[UNTRUSTED_DATA]This is a test comment[/UNTRUSTED_DATA]", comment["message"])
}

func createTaskWithLog(t *testing.T, hiveClient *thehive.APIClient) map[string]interface{} {
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Test case for task logs"
	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)

	testTask := testutils.MockInputTask()
	testTask.Title = "Test Task for logs"
	createdTask, _, err := hiveClient.TaskAPI.CreateTaskInCase(authContext, createdCase.UnderscoreId).
		InputCreateTask(*testTask).Execute()
	require.NoError(t, err)

	logInput := thehive.InputCreateLog{
		Message: "This is a test log entry",
	}
	_, _, err = hiveClient.TaskLogAPI.CreateTaskLog(authContext, createdTask.UnderscoreId).InputCreateLog(logInput).Execute()
	require.NoError(t, err)

	return map[string]interface{}{
		"case_id": createdCase.UnderscoreId,
		"task_id": createdTask.UnderscoreId,
	}
}

func TestSearchTaskTasKLogs(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	creationResult := createTaskWithLog(t, hiveClient)

	mcpClient := newSearchClient(t)

	tasksData := searchRows(t, mcpClient, map[string]any{
		"entity-type":        types.EntityTypeTask,
		"extra-columns":      []string{"_id", "title"},
		"additional-queries": []string{"task-logs"},
	})
	require.NotEmpty(t, tasksData)

	// Shared TheHive instance may hold tasks from other tests; assert on this test's own task, not the count.
	var found bool
	for _, taskInterface := range tasksData {
		task := taskInterface.(map[string]any)
		if task["_id"].(string) != creationResult["task_id"] {
			continue
		}
		found = true

		logs, ok := task["task-logs"].([]any)
		require.True(t, ok)
		require.Len(t, logs, 1)

		log := logs[0].(map[string]any)
		require.Equal(t, "[UNTRUSTED_DATA]This is a test log entry[/UNTRUSTED_DATA]", log["message"])
	}
	require.True(t, found, "search results should include the task created by this test")
}

func TestSearchCaseTemplates(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	for _, name := range []string{"Phishing-Search-Test", "Malware-Search-Test"} {
		input := testutils.MockInputCaseTemplate()
		input.Name = name
		_, _, err := hiveClient.CaseTemplateAPI.CreateCaseTemplate(authContext).InputCreateCaseTemplate(*input).Execute()
		require.NoError(t, err)
	}

	mcpClient := newSearchClient(t)

	templatesData := searchRows(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeCaseTemplate,
		"filters": map[string]any{
			"_like": map[string]any{
				"_field": "name",
				"_value": "Phishing*",
			},
		},
		"extra-columns": []string{"_id", "name", "displayName"},
	})
	require.Len(t, templatesData, 1)

	template, ok := templatesData[0].(map[string]any)
	require.True(t, ok)
	// DL-6006: "name" is free text, so it is wrapped (the agent uses "_id").
	require.Equal(t, "[UNTRUSTED_DATA]Phishing-Search-Test[/UNTRUSTED_DATA]", template["name"])
}

func TestSearchPages(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Case with Pages for Search"

	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdCase)

	inputPage := thehive.InputCreatePage{
		Title:    "Searchable Investigation Page",
		Content:  "## Notes\nSome investigation content.",
		Category: "Default",
	}
	_, _, err = hiveClient.PageAPI.CreateAPageInACase(authContext, createdCase.UnderscoreId).InputCreatePage(inputPage).Execute()
	require.NoError(t, err)

	mcpClient := newSearchClient(t)

	pagesData := searchRows(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypePage,
		"filters": map[string]any{
			"_eq": map[string]any{
				"_field": "category",
				"_value": "Default",
			},
		},
		"extra-columns": []string{"_id", "title", "category"},
	})
	require.GreaterOrEqual(t, len(pagesData), 1)

	page, ok := pagesData[0].(map[string]any)
	require.True(t, ok)
	// DL-6006: "category" is a user-defined label, so it is wrapped.
	require.Equal(t, "[UNTRUSTED_DATA]Default[/UNTRUSTED_DATA]", page["category"])
}
