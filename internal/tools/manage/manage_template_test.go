package manage_test

import (
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestManageCreateCaseTemplate(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

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

	authContext := testutils.GetAuthContext(t)
	fetchedTemplate, _, err := hiveClient.CaseTemplateAPI.GetCaseTemplate(authContext, templateID).Execute()
	require.NoError(t, err)
	require.Equal(t, "Test-MCP-Template", fetchedTemplate.Name)
}

func TestManageUpdateCaseTemplate(t *testing.T) {
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)
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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t)

	authContext := testutils.GetAuthContext(t)

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
	testutils.Parallel(t)
	hiveClient := testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, testutils.PermissionsFixture(t, "analyst.yaml"))

	authContext := testutils.GetAuthContext(t)

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
	testutils.Parallel(t)
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClientWithPermissions(t, testutils.PermissionsFixture(t, "analyst.yaml"))

	result := testutils.CallTool(t, mcpClient, testToolName, map[string]any{
		testArgOperation:  testOpCreate,
		testArgEntityType: types.EntityTypeCaseTemplate,
		testArgEntityData: map[string]any{
			"name": "Analyst-Created-Template",
		},
	})
	testutils.RequirePermissionDenied(t, result)
}
