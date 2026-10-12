package search_test

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func createTestAlert(t *testing.T, hiveClient *thehive.APIClient, title string, severity int32, tags []string) map[string]any {
	t.Helper()

	testAlert := testutils.MockInputAlert()
	testAlert.Title = title
	testAlert.Severity = &severity
	testAlert.Tags = tags
	testAlert.SourceRef = "test-" + title

	authContext := testutils.GetAuthContext(t)
	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*testAlert).Execute()
	require.NoError(t, err)
	require.NotNil(t, createdAlert)

	return map[string]any{
		tID:       createdAlert.UnderscoreId,
		tTitle:    createdAlert.Title,
		tSeverity: createdAlert.Severity,
	}
}

func createTestCase(t *testing.T, hiveClient *thehive.APIClient, title string, severity int32, status string, assignee string) {
	t.Helper()

	testCase := testutils.MockInputCase()
	testCase.Title = title
	testCase.Severity = &severity

	testCase.Status = &status
	if assignee != "" {
		testCase.Assignee = &assignee
	} else {
		testCase.Assignee = nil // Explicitly set to nil to remove the default assignee
	}

	authContext := testutils.GetAuthContext(t)
	createdCase, resp, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	slog.Info("Create case response", "response", resp)
	require.NoError(t, err)
	require.NotNil(t, createdCase)
}

func createTestCaseWithTaskAndAlert(t *testing.T, hiveClient *thehive.APIClient) map[string]any {
	t.Helper()

	testCase := testutils.MockInputCase()
	testCase.Title = "Test case with tasks"
	testAlert := testutils.MockInputAlert()
	testAlert.Title = "Test alert 1"
	authContext := testutils.GetAuthContext(t)
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

	return map[string]any{
		tCaseID:    createdCase.UnderscoreId,
		"alert_id": createdAlert.UnderscoreId,
	}
}

func TestSearchCasesBySeverityAndStatus(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestCase(t, hiveClient, "High severity open case", 3, "New", "")
	createTestCase(t, hiveClient, "Low severity open case", 1, "New", "")
	createTestCase(t, hiveClient, "High severity in progress case", 3, "InProgress", "")

	mcpClient := newSearchClient(t)

	casesData := searchRows(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeCase,
		pFilters: map[string]any{
			tAnd: []any{
				map[string]any{
					tGte: map[string]any{
						tField: tSeverity,
						tValue: 3,
					},
				},
				map[string]any{
					tEq: map[string]any{
						tField: tStatus,
						tValue: "New",
					},
				},
			},
		},
		pExtraColumns: []string{tID, tTitle, tSeverity, tStatus},
	})
	require.Len(t, casesData, 1)

	caseData, ok := casesData[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "[UNTRUSTED_DATA]High severity open case[/UNTRUSTED_DATA]", caseData[tTitle])
	require.InDelta(t, float64(3), caseData[tSeverity], 0)
}

func TestSearchAlertsWithDateRange(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Recent alert", 2, []string{"recent"})
	time.Sleep(100 * time.Millisecond) // distinct timestamps
	createTestAlert(t, hiveClient, "Another recent alert", 2, []string{"recent"})

	now := time.Now()
	fromTime := now.Add(-1 * time.Hour).UnixMilli()
	toTime := now.UnixMilli()

	mcpClient := newSearchClient(t)

	alertsData := searchRows(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeAlert,
		pFilters: map[string]any{
			tBetween: map[string]any{
				tField:  tCreatedAt,
				"_from": fromTime,
				"_to":   toTime,
			},
		},
		pExtraColumns: []string{tID, tTitle, tCreatedAt},
	})
	require.GreaterOrEqual(t, len(alertsData), 2)
}

func TestSearchAlertsWithMultipleTags(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Phishing alert", 3, []string{tPhishing, "email"})
	createTestAlert(t, hiveClient, "Malware alert", 3, []string{tMalware, "endpoint"})
	createTestAlert(t, hiveClient, "Network alert", 2, []string{tNetwork, "firewall"})

	mcpClient := newSearchClient(t)

	alertsData := searchRows(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeAlert,
		pFilters: map[string]any{
			tOr: []any{
				map[string]any{
					tIn: map[string]any{
						tField:  tTags,
						tValues: []any{tPhishing, tMalware},
					},
				},
			},
		},
		pExtraColumns: []string{tID, tTitle, tTags, tSeverity},
		"sort-by":     tSeverity,
	})
	require.Len(t, alertsData, 2)
}

func TestSearchCasesWithAssigneeAndSorting(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	// Assign to this test's own dedicated org user (the only member of the
	// per-test org; the shared admin is not a member).
	assignee := testutils.TestUserLogin(t)
	createTestCase(t, hiveClient, "Admin's case 1", 2, "InProgress", assignee)
	time.Sleep(50 * time.Millisecond)
	createTestCase(t, hiveClient, "Admin's case 2", 3, "InProgress", assignee)

	mcpClient := newSearchClient(t)

	casesData := searchRows(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeCase,
		pFilters: map[string]any{
			tAnd: []any{
				map[string]any{
					tEq: map[string]any{
						tField: "assignee",
						tValue: assignee,
					},
				},
				map[string]any{
					tEq: map[string]any{
						tField: tStatus,
						tValue: "InProgress",
					},
				},
			},
		},
		pExtraColumns: []string{tID, tTitle, "assignee", tCreatedAt},
		"sort-order":  "asc",
	})
	require.Len(t, casesData, 2)

	firstCase, ok := casesData[0].(map[string]any)
	require.True(t, ok)
	secondCase, ok := casesData[1].(map[string]any)
	require.True(t, ok)

	require.Equal(t, "[UNTRUSTED_DATA]Admin's case 1[/UNTRUSTED_DATA]", firstCase[tTitle])
	require.Equal(t, "[UNTRUSTED_DATA]Admin's case 2[/UNTRUSTED_DATA]", secondCase[tTitle])
}

func TestSearchAlertsWithComplexOrConditions(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Critical alert", 4, []string{"critical"})
	createTestAlert(t, hiveClient, "High alert", 3, []string{"high"})
	createTestAlert(t, hiveClient, "Medium alert", 2, []string{"medium"})
	createTestAlert(t, hiveClient, "Low alert", 1, []string{"low"})

	mcpClient := newSearchClient(t)

	alertsData := searchRows(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeAlert,
		pFilters: map[string]any{
			tOr: []any{
				map[string]any{
					tEq: map[string]any{
						tField: tSeverity,
						tValue: 4,
					},
				},
				map[string]any{
					tEq: map[string]any{
						tField: tSeverity,
						tValue: 3,
					},
				},
			},
		},
		pExtraColumns: []string{tID, tTitle, tSeverity},
		"sort-by":     tSeverity,
	})
	require.Len(t, alertsData, 2)

	for _, alertAny := range alertsData {
		alert, ok := alertAny.(map[string]any)
		require.True(t, ok)
		severityValue, ok := alert[tSeverity].(float64)
		require.True(t, ok)
		require.GreaterOrEqual(t, int(severityValue), 3, "Only high (3) and critical (4) severity alerts should be returned")
	}
}

func TestSearchTasksWithLimit(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	authContext := testutils.GetAuthContext(t)
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
		pEntityType: types.EntityTypeTask,
		"limit":     3,
	})
	require.Len(t, tasksData, 3, "Should return exactly 3 tasks as per limit")
}

// extra-columns adds to the entity defaults. It used to replace them, so asking
// for one more column silently dropped severity, status and _createdAt — the
// caller requesting extra data received less of it.
func TestExtraColumnsExtendDefaults(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Test alert for column extension", 2, []string{"test"})

	mcpClient := newSearchClient(t)

	alertsData := searchRows(t, mcpClient, map[string]any{
		pEntityType:   types.EntityTypeAlert,
		pExtraColumns: []string{tTags},
	})
	require.GreaterOrEqual(t, len(alertsData), 1)

	alertData, ok := alertsData[0].(map[string]any)
	require.True(t, ok)

	require.Contains(t, alertData, tTags, "the requested column must be projected")

	// The alert defaults survive alongside it.
	for _, column := range types.DefaultFields[types.EntityTypeAlert] {
		require.Contains(t, alertData, column, "requesting a column must not drop the default %s", column)
	}
}

func TestSearchWithAnalystPermissions(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)
	authContext := testutils.GetAuthContext(t)

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

	mcpClient := testutils.GetMCPTestClientWithPermissions(t, testutils.PermissionsFixture(t, "analyst.yaml"))

	alertsData := searchRows(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeAlert,
	})

	visibleIDs := make(map[string]bool)

	for _, alertInterface := range alertsData {
		alert, ok := alertInterface.(map[string]any)
		require.True(t, ok)
		alertID, ok := alert[tID].(string)
		require.True(t, ok)

		visibleIDs[alertID] = true
	}

	require.True(t, visibleIDs[createdAlert1.UnderscoreId], "Alert with TLP=2, PAP=2 should be visible")
	require.False(t, visibleIDs[createdAlert2.UnderscoreId], "Alert with TLP=3 should NOT be visible")
	require.False(t, visibleIDs[createdAlert3.UnderscoreId], "Alert with PAP=3 should NOT be visible")
}

func TestSearchWithReadOnlyPermissions(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)
	authContext := testutils.GetAuthContext(t)

	alert := testutils.MockInputAlert()
	alert.Title = "ReadOnly Search Test Alert"
	alert.SourceRef = "test-readonly-search-001"
	severity := int32(2)
	alert.Severity = &severity
	createdAlert, _, err := hiveClient.AlertAPI.CreateAlert(authContext).InputCreateAlert(*alert).Execute()
	require.NoError(t, err)

	mcpClient := testutils.GetMCPTestClientWithPermissions(t, "")

	result := callSearch(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeAlert,
	})
	require.False(t, result.IsError, "Search should succeed with read-only permissions")

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	alertsData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(alertsData), 1, "Should find at least one alert")

	found := false

	for _, alertInterface := range alertsData {
		alert, ok := alertInterface.(map[string]any)
		require.True(t, ok)
		alertID, ok := alert[tID].(string)
		require.True(t, ok)

		if alertID == createdAlert.UnderscoreId {
			found = true
			break
		}
	}

	require.True(t, found, "Should find our test alert")
}
