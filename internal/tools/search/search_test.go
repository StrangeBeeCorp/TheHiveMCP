package search_test

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test alert with specific fields
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

// Helper function to create a test case with specific fields
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

// TestSearchCasesBySeverityAndStatus tests searching cases with multiple filter conditions
func TestSearchCasesBySeverityAndStatus(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Create test cases with different severities and statuses
	createTestCase(t, hiveClient, "High severity open case", 3, "New", "")
	createTestCase(t, hiveClient, "Low severity open case", 1, "New", "")
	createTestCase(t, hiveClient, "High severity in progress case", 3, "InProgress", "")

	samplingHandler := testutils.SamplingHandlerCreateMessageFromStringResponse(
		`{
			"raw_filters": {
				"_and": [
					{
						"_gte": {
							"_field": "severity",
							"_value": 3
						}
					},
					{
						"_eq": {
							"_field": "status",
							"_value": "New"
						}
					}
				]
			},
			"sort_by": "_createdAt",
			"sort_order": "desc",
			"num_results": 10,
			"kept_columns": ["_id", "title", "severity", "status"],
			"extra_data": [],
			"additional_queries": []
		}`,
	)

	mcpClient := testutils.GetMCPTestClient(
		t,
		samplingHandler,
		testutils.DummyElicitationAccept,
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type":   "case",
				"query":         "high severity cases with status New",
				"extra-columns": []string{"_id", "title", "severity", "status"},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	casesData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.Len(t, casesData, 1)

	caseData, ok := casesData[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "High severity open case", caseData["title"])
	require.Equal(t, float64(3), caseData["severity"])
}

// TestSearchAlertsWithDateRange tests searching alerts created within a date range
func TestSearchAlertsWithDateRange(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Create test alerts
	createTestAlert(t, hiveClient, "Recent alert", 2, []string{"recent"})
	time.Sleep(100 * time.Millisecond) // Ensure different timestamps
	createTestAlert(t, hiveClient, "Another recent alert", 2, []string{"recent"})

	now := time.Now()
	fromTime := now.Add(-1 * time.Hour).UnixMilli()
	toTime := now.UnixMilli()

	samplingHandler := testutils.SamplingHandlerCreateMessageFromStringResponse(
		`{
			"raw_filters": {
				"_between": {
					"_field": "_createdAt",
					"_from": ` + fmt.Sprintf("%d", fromTime) + `,
					"_to": ` + fmt.Sprintf("%d", toTime) + `
				}
			},
			"sort_by": "_createdAt",
			"sort_order": "desc",
			"num_results": 10,
			"kept_columns": ["_id", "title", "_createdAt"],
			"extra_data": [],
			"additional_queries": []
		}`,
	)

	mcpClient := testutils.GetMCPTestClient(
		t,
		samplingHandler,
		testutils.DummyElicitationAccept,
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type":   "alert",
				"query":         "alerts from the last hour",
				"extra-columns": []string{"_id", "title", "_createdAt"},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	alertsData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(alertsData), 2)
}

// TestSearchAlertsWithMultipleTags tests searching alerts using the _in operator for tags
func TestSearchAlertsWithMultipleTags(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Create alerts with different tags
	createTestAlert(t, hiveClient, "Phishing alert", 3, []string{"phishing", "email"})
	createTestAlert(t, hiveClient, "Malware alert", 3, []string{"malware", "endpoint"})
	createTestAlert(t, hiveClient, "Network alert", 2, []string{"network", "firewall"})

	samplingHandler := testutils.SamplingHandlerCreateMessageFromStringResponse(
		`{
			"raw_filters": {
				"_or": [
					{
						"_in": {
							"_field": "tags",
							"_values": ["phishing", "malware"]
						}
					}
				]
			},
			"sort_by": "severity",
			"sort_order": "desc",
			"num_results": 10,
			"kept_columns": ["_id", "title", "tags", "severity"],
			"extra_data": [],
			"additional_queries": []
		}`,
	)

	mcpClient := testutils.GetMCPTestClient(
		t,
		samplingHandler,
		testutils.DummyElicitationAccept,
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type":   "alert",
				"query":         "alerts tagged with phishing or malware",
				"extra-columns": []string{"_id", "title", "tags", "severity"},
				"sort-by":       "severity",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	alertsData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.Len(t, alertsData, 2)
}

// TestSearchCasesWithAssigneeAndSorting tests searching cases assigned to specific user
func TestSearchCasesWithAssigneeAndSorting(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Create cases with different assignees (using admin user since test users don't exist)
	createTestCase(t, hiveClient, "Admin's case 1", 2, "InProgress", "admin@thehive.local")
	time.Sleep(50 * time.Millisecond)
	createTestCase(t, hiveClient, "Admin's case 2", 3, "InProgress", "admin@thehive.local")
	// Note: TheHive assigns the creator as default assignee even when we set nil, so all cases will show admin as assignee

	samplingHandler := testutils.SamplingHandlerCreateMessageFromStringResponse(
		`{
			"raw_filters": {
				"_and": [
					{
						"_eq": {
							"_field": "assignee",
							"_value": "admin@thehive.local"
						}
					},
					{
						"_eq": {
							"_field": "status",
							"_value": "InProgress"
						}
					}
				]
			},
			"sort_by": "_createdAt",
			"sort_order": "asc",
			"num_results": 10,
			"kept_columns": ["_id", "title", "assignee", "_createdAt"],
			"extra_data": [],
			"additional_queries": []
		}`,
	)

	mcpClient := testutils.GetMCPTestClient(
		t,
		samplingHandler,
		testutils.DummyElicitationAccept,
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type":   "case",
				"query":         "in progress cases assigned to admin@thehive.local",
				"extra-columns": []string{"_id", "title", "assignee", "_createdAt"},
				"sort-order":    "asc",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	casesData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.Len(t, casesData, 2) // Should match both cases assigned to admin

	// Verify sorting (oldest first with asc order)
	firstCase := casesData[0].(map[string]any)
	secondCase := casesData[1].(map[string]any)
	require.Equal(t, "Admin's case 1", firstCase["title"])
	require.Equal(t, "Admin's case 2", secondCase["title"])
}

// TestSearchAlertsWithComplexOrConditions tests using _or with multiple severity levels
func TestSearchAlertsWithComplexOrConditions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Create alerts with different severities
	createTestAlert(t, hiveClient, "Critical alert", 4, []string{"critical"})
	createTestAlert(t, hiveClient, "High alert", 3, []string{"high"})
	createTestAlert(t, hiveClient, "Medium alert", 2, []string{"medium"})
	createTestAlert(t, hiveClient, "Low alert", 1, []string{"low"})

	samplingHandler := testutils.SamplingHandlerCreateMessageFromStringResponse(
		`{
			"raw_filters": {
				"_or": [
					{
						"_eq": {
							"_field": "severity",
							"_value": 4
						}
					},
					{
						"_eq": {
							"_field": "severity",
							"_value": 3
						}
					}
				]
			},
			"sort_by": "severity",
			"sort_order": "desc",
			"num_results": 10,
			"kept_columns": ["_id", "title", "severity"],
			"extra_data": [],
			"additional_queries": []
		}`,
	)

	mcpClient := testutils.GetMCPTestClient(
		t,
		samplingHandler,
		testutils.DummyElicitationAccept,
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type":   "alert",
				"query":         "critical or high severity alerts",
				"extra-columns": []string{"_id", "title", "severity"},
				"sort-by":       "severity",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	alertsData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.Len(t, alertsData, 2)

	// Verify only high and critical alerts are returned
	for _, alertAny := range alertsData {
		alert := alertAny.(map[string]any)
		severity := int(alert["severity"].(float64))
		require.GreaterOrEqual(t, severity, 3, "Only high (3) and critical (4) severity alerts should be returned")
	}
}

// TestSearchTasksWithLimit tests searching tasks with a custom limit
func TestSearchTasksWithLimit(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// First create a case
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	testCase := testutils.MockInputCase()
	testCase.Title = "Test case for tasks"
	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)

	// Create multiple tasks
	for i := 1; i <= 5; i++ {
		testTask := testutils.MockInputTask()
		testTask.Title = fmt.Sprintf("Task %d", i)
		_, _, err := hiveClient.TaskAPI.CreateTaskInCase(authContext, createdCase.UnderscoreId).
			InputCreateTask(*testTask).Execute()
		require.NoError(t, err)
	}

	samplingHandler := testutils.SamplingHandlerCreateMessageFromStringResponse(
		`{
			"raw_filters": {
				"_any": ""
			},
			"sort_by": "_createdAt",
			"sort_order": "desc",
			"num_results": 3,
			"kept_columns": ["_id", "title"],
			"extra_data": [],
			"additional_queries": []
		}`,
	)

	mcpClient := testutils.GetMCPTestClient(
		t,
		samplingHandler,
		testutils.DummyElicitationAccept,
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type": "task",
				"query":       "show me the latest tasks",
				"limit":       3,
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	tasksData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.Len(t, tasksData, 3, "Should return exactly 3 tasks as per limit")
}

// TestKeptColumnsOverrideExtraColumns tests that kept_columns from handler takes priority over extra-columns from tool call
func TestKeptColumnsOverrideExtraColumns(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Create a test alert
	createTestAlert(t, hiveClient, "Test alert for column override", 2, []string{"test"})

	// Handler specifies only specific columns in kept_columns
	samplingHandler := testutils.SamplingHandlerCreateMessageFromStringResponse(
		`{
			"raw_filters": {
				"_any": ""
			},
			"sort_by": "_createdAt",
			"sort_order": "desc",
			"num_results": 10,
			"kept_columns": ["_id", "title"],
			"extra_data": [],
			"additional_queries": []
		}`,
	)

	mcpClient := testutils.GetMCPTestClient(
		t,
		samplingHandler,
		testutils.DummyElicitationAccept,
	)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type": "alert",
				"query":       "show me alerts",
				// Request additional columns that should be ignored by handler's kept_columns
				"extra-columns": []string{"_id", "title", "severity", "tags", "_createdAt"},
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	alertsData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(alertsData), 1)

	// Verify that only the columns from kept_columns are returned
	alertData := alertsData[0].(map[string]any)

	// These should be present (from kept_columns)
	require.Contains(t, alertData, "_id")
	require.Contains(t, alertData, "title")

	// These should NOT be present (not in kept_columns, even though requested in extra-columns)
	require.NotContains(t, alertData, "severity", "severity should not be present as it's not in kept_columns")
	require.NotContains(t, alertData, "tags", "tags should not be present as it's not in kept_columns")
	require.NotContains(t, alertData, "_createdAt", "_createdAt should not be present as it's not in kept_columns")

	// Verify we only have the expected number of columns
	require.Len(t, alertData, 2, "Should only have 2 columns as specified in kept_columns")
}

// TestSearchWithAnalystPermissions tests that analyst permissions filter results by TLP and PAP
func TestSearchWithAnalystPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	// Create alerts with different TLP/PAP levels
	// Alert 1: TLP=2, PAP=2 (should be visible)
	alert1 := testutils.MockInputAlert()
	tlp1 := int32(2)
	pap1 := int32(2)
	alert1.Tlp = &tlp1
	alert1.Pap = &pap1
	alert1.Title = "Alert TLP2 PAP2"
	alert1.SourceRef = "test-analyst-search-001"
	createdAlert1, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*alert1).Execute()
	require.NoError(t, err)

	// Alert 2: TLP=3, PAP=1 (should NOT be visible - TLP too high)
	alert2 := testutils.MockInputAlert()
	tlp2 := int32(3)
	pap2 := int32(1)
	alert2.Tlp = &tlp2
	alert2.Pap = &pap2
	alert2.Title = "Alert TLP3 PAP1"
	alert2.SourceRef = "test-analyst-search-002"
	createdAlert2, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*alert2).Execute()
	require.NoError(t, err)

	// Alert 3: TLP=1, PAP=3 (should NOT be visible - PAP too high)
	alert3 := testutils.MockInputAlert()
	tlp3 := int32(1)
	pap3 := int32(3)
	alert3.Tlp = &tlp3
	alert3.Pap = &pap3
	alert3.Title = "Alert TLP1 PAP3"
	alert3.SourceRef = "test-analyst-search-003"
	createdAlert3, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*alert3).Execute()
	require.NoError(t, err)

	// Use analyst permissions client
	samplingHandler := testutils.SamplingHandlerCreateMessageFromStringResponse(
		`{
			"raw_filters": {
				"_any": ""
			},
			"sort_by": "_createdAt",
			"sort_order": "desc",
			"num_results": 10,
			"kept_columns": ["_id", "title"],
			"extra_data": [],
			"additional_queries": []
		}`,
	)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, samplingHandler, testutils.DummyElicitationAccept, "../../../docs/examples/permissions/analyst.yaml")

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type": "alert",
				"query":       "show me all alerts",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	alertsData, ok := structuredData["results"].([]any)
	require.True(t, ok)

	// Check that only alert1 is visible (TLP<=2 and PAP<=2)
	visibleIDs := make(map[string]bool)
	for _, alertInterface := range alertsData {
		alert := alertInterface.(map[string]any)
		visibleIDs[alert["_id"].(string)] = true
	}

	require.True(t, visibleIDs[createdAlert1.UnderscoreId], "Alert with TLP=2, PAP=2 should be visible")
	require.False(t, visibleIDs[createdAlert2.UnderscoreId], "Alert with TLP=3 should NOT be visible")
	require.False(t, visibleIDs[createdAlert3.UnderscoreId], "Alert with PAP=3 should NOT be visible")
}

// TestSearchWithReadOnlyPermissions tests that read-only permissions still allow searching
func TestSearchWithReadOnlyPermissions(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())

	// Create a test alert
	alert := testutils.MockInputAlert()
	alert.Title = "ReadOnly Search Test Alert"
	alert.SourceRef = "test-readonly-search-001"
	severity := int32(2)
	alert.Severity = &severity
	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*alert).Execute()
	require.NoError(t, err)

	// Use read-only permissions client (default permissions)
	samplingHandler := testutils.SamplingHandlerCreateMessageFromStringResponse(
		`{
			"raw_filters": {
				"_any": ""
			},
			"sort_by": "_createdAt",
			"sort_order": "desc",
			"num_results": 10,
			"kept_columns": ["_id", "title"],
			"extra_data": [],
			"additional_queries": []
		}`,
	)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, samplingHandler, testutils.DummyElicitationAccept, "")

	// Test: Search should succeed with read-only permissions
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type": "alert",
				"query":       "show me alerts",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Search should succeed with read-only permissions")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	alertsData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(alertsData), 1, "Should find at least one alert")

	// Verify our test alert is in the results
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
