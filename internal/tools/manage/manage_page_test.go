package manage_test

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestManageCreatePageInCase(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

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
	testutils.Parallel(t)
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	createdPage := createPageInCase(t, hiveClient, "Case for Page Update", thehive.InputCreatePage{
		Title:    "Original Page Title",
		Content:  "## Original\ncontent.",
		Category: testCategoryDefault,
	})

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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	createdPage := createPageInCase(t, hiveClient, "Case for Page Deletion", thehive.InputCreatePage{
		Title:    "Page to Delete",
		Content:  "This page will be deleted.",
		Category: testCategoryDefault,
	})

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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, testutils.PermissionsFixture(t, "analyst.yaml"))

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

	createdPage := createPageInCase(t, hiveClient, "Case for Analyst Page Permission Test", thehive.InputCreatePage{
		Title:    "Page for Analyst Delete Test",
		Content:  "Content",
		Category: testCategoryDefault,
	})

	deleteResult := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpDelete,
		testArgEntityType: types.EntityTypePage,
		testArgEntityIDs:  []string{createdPage.UnderscoreId},
	})
	testutils.RequirePermissionDenied(t, deleteResult)
}
