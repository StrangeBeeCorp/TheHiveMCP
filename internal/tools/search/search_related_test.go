package search_test

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestSearchExtraDataAndAdditionalQueries(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	creationResult := createTestCaseWithTaskAndAlert(t, hiveClient)

	mcpClient := newSearchClient(t)

	structuredData := searchStructured(t, mcpClient, map[string]any{
		pEntityType:        types.EntityTypeCase,
		pExtraColumns:      []string{tID, tTitle},
		"extra-data":       []string{"alerts"},
		pAdditionalQueries: []string{tTasks},
	})

	casesData, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.Len(t, casesData, 1)

	caseData, ok := casesData[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, creationResult[tCaseID], caseData[tID])
	require.Equal(t, "[UNTRUSTED_DATA]Test case with tasks[/UNTRUSTED_DATA]", caseData[tTitle])

	restults, ok := structuredData["results"].([]any)
	require.True(t, ok)
	require.Len(t, restults, 1)

	firstResult, ok := restults[0].(map[string]any)
	require.True(t, ok)
	extraData, ok := firstResult["extraData"].(map[string]any)
	require.True(t, ok)

	alertsData, ok := extraData["alerts"].([]any)
	require.True(t, ok)
	require.Len(t, alertsData, 1)

	alert, ok := alertsData[0].(map[string]any)
	require.True(t, ok)
	// DL-6006: "type" is an open ingestion-controlled label, so it is wrapped.
	require.Equal(t, "[UNTRUSTED_DATA]test[/UNTRUSTED_DATA]", alert["type"])
	require.Equal(t, "[UNTRUSTED_DATA]test[/UNTRUSTED_DATA]", alert["source"])

	tasks, ok := firstResult[tTasks].([]any)
	require.True(t, ok)
	require.Len(t, tasks, 1)

	task, ok := tasks[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "[UNTRUSTED_DATA]Test Task[/UNTRUSTED_DATA]", task[tTitle])
}

func createTestCaseWithComment(t *testing.T, hiveClient *thehive.APIClient) map[string]any {
	t.Helper()

	authContext := testutils.GetAuthContext(t)
	testCase := testutils.MockInputCase()
	testCase.Title = "Test case for comment"
	createdCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*testCase).Execute()
	require.NoError(t, err)

	commentInput := thehive.InputComment{
		Message: "This is a test comment",
	}
	_, _, err = hiveClient.CommentAPI.CreateCommentInCase(authContext, createdCase.UnderscoreId).InputComment(commentInput).Execute()
	require.NoError(t, err)

	return map[string]any{
		tCaseID: createdCase.UnderscoreId,
	}
}

func TestSearchAdditionalQueriesComments(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)
	creationResult := createTestCaseWithComment(t, hiveClient)

	mcpClient := newSearchClient(t)

	casesData := searchRows(t, mcpClient, map[string]any{
		pEntityType:        types.EntityTypeCase,
		pExtraColumns:      []string{tID, tTitle},
		pAdditionalQueries: []string{"comments"},
	})
	require.Len(t, casesData, 1)

	caseData, ok := casesData[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, creationResult[tCaseID], caseData[tID])
	require.Equal(t, "[UNTRUSTED_DATA]Test case for comment[/UNTRUSTED_DATA]", caseData[tTitle])

	comments, ok := caseData["comments"].([]any)
	require.True(t, ok)
	require.Len(t, comments, 1)

	comment, ok := comments[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "[UNTRUSTED_DATA]This is a test comment[/UNTRUSTED_DATA]", comment["message"])
}

func createTaskWithLog(t *testing.T, hiveClient *thehive.APIClient) map[string]any {
	t.Helper()

	authContext := testutils.GetAuthContext(t)
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

	return map[string]any{
		tCaseID:   createdCase.UnderscoreId,
		"task_id": createdTask.UnderscoreId,
	}
}

func TestSearchTaskTasKLogs(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)
	creationResult := createTaskWithLog(t, hiveClient)

	mcpClient := newSearchClient(t)

	tasksData := searchRows(t, mcpClient, map[string]any{
		pEntityType:        types.EntityTypeTask,
		pExtraColumns:      []string{tID, tTitle},
		pAdditionalQueries: []string{"task-logs"},
	})
	require.NotEmpty(t, tasksData)

	// Shared TheHive instance may hold tasks from other tests; assert on this test's own task, not the count.
	var found bool

	for _, taskInterface := range tasksData {
		task, ok := taskInterface.(map[string]any)
		require.True(t, ok)
		taskID, ok := task[tID].(string)
		require.True(t, ok)

		if taskID != creationResult["task_id"] {
			continue
		}

		found = true

		logs, ok := task["task-logs"].([]any)
		require.True(t, ok)
		require.Len(t, logs, 1)

		log, ok := logs[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "[UNTRUSTED_DATA]This is a test log entry[/UNTRUSTED_DATA]", log["message"])
	}

	require.True(t, found, "search results should include the task created by this test")
}
