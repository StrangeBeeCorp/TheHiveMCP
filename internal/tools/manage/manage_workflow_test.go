package manage_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestManagePromoteAlert(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)

	caseIDs := createCases(t, hiveClient, 2, "Case %d for Merging")

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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, testutils.PermissionsFixture(t, "analyst.yaml"))

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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, testutils.PermissionsFixture(t, "analyst.yaml"))

	caseIDs := createCases(t, hiveClient, 2, "Case %d for Analyst Merge Test")

	structuredData := manageStructured(t, mcpClient, map[string]any{
		testArgOperation:  testOpMerge,
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  caseIDs,
	})
	require.Equal(t, testOpMerge, structuredData[testArgOperation])
}

func TestManagePromoteWithReadOnlyPermissions(t *testing.T) {
	testutils.Parallel(t)
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, "")

	result := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpPromote,
		testArgEntityType: types.EntityTypeAlert,
		testArgEntityIDs:  []string{testEntityID},
	})
	testutils.RequirePermissionDenied(t, result)
}

func TestManageMergeWithReadOnlyPermissions(t *testing.T) {
	testutils.Parallel(t)
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, "")

	result := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpMerge,
		testArgEntityType: types.EntityTypeCase,
		testArgEntityIDs:  []string{testEntityID, "~456"},
	})
	testutils.RequirePermissionDenied(t, result)
}
