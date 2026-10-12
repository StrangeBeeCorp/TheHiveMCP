package manage_test

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestManageCreateProcedureInCase(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
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
