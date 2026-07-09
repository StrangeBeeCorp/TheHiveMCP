package resource_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
)

func TestGetResourceCatalog(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "")

	requireResourceMeta(t, structuredData, "hive://catalog", "Resource Catalog", mimeJSON)

	data, ok := structuredData[fieldData].(map[string]any)
	require.True(t, ok)

	categoryNames := collectNames(t, data, fieldCategories, fieldName)

	require.Contains(t, categoryNames, "config")
	require.Contains(t, categoryNames, "schema")
	require.Contains(t, categoryNames, "metadata")
	require.Contains(t, categoryNames, "docs")
}

func TestGetResourceBrowseSchemaCategory(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "schema")

	require.Equal(t, "hive://schema", structuredData[argURI])

	resourceNames := collectNames(t, structuredData, fieldResources, fieldName)

	require.Contains(t, resourceNames, "Alert Output Schema")
	require.Contains(t, resourceNames, "Case Output Schema")
	require.Contains(t, resourceNames, "Task Output Schema")
	require.Contains(t, resourceNames, "Observable Output Schema")
}

func TestGetResourceFetchAlertSchema(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "hive://schema/alert")

	requireResourceMeta(t, structuredData, "hive://schema/alert", "Alert Output Schema", mimeJSON)

	data, ok := structuredData[fieldData].(map[string]any)
	require.True(t, ok)

	fields, ok := data["properties"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, fields)

	fieldNames := make([]string, 0)
	for name := range fields {
		fieldNames = append(fieldNames, name)
	}

	require.Contains(t, fieldNames, "title")
	require.Contains(t, fieldNames, "severity")
	require.Contains(t, fieldNames, "type")
	require.Contains(t, fieldNames, "source")
}

func TestGetResourceFetchCurrentUser(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "hive://config/current-user")

	requireResourceMeta(t, structuredData, "hive://config/current-user", "Current User", mimeJSON)

	data, ok := structuredData[fieldData].(map[string]any)
	require.True(t, ok)

	require.Contains(t, data, "login")
	require.Contains(t, data, "name")
	require.NotEmpty(t, data["login"])

	// The MCP client authenticates as this test's dedicated per-test-org user.
	require.Equal(t, testutils.TestUserLogin(t), data["login"])
}

func TestGetResourceFetchDocumentation(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "hive://docs/entities/case")

	requireResourceMeta(t, structuredData, "hive://docs/entities/case", "Case Documentation", "text/plain")

	data, ok := structuredData["content"].(string)
	require.True(t, ok)
	require.NotEmpty(t, data)

	require.Contains(t, data, "A case is a structured entity used to track, investigate,")
}

func TestGetResourceBrowseMetadataCategory(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "metadata")

	require.Equal(t, "hive://metadata", structuredData[argURI])

	subcategoryNames := collectNames(t, structuredData, fieldSubcategories, fieldName)

	require.Contains(t, subcategoryNames, "entities")
	require.Contains(t, subcategoryNames, "automation")
	require.Contains(t, subcategoryNames, "organisation")
}

func TestGetResourceFetchCaseStatuses(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	structuredData := getResourceStructured(t, mcpClient, "hive://metadata/entities/case/statuses")

	require.Equal(t, "hive://metadata/entities/case/statuses", structuredData[argURI])
	require.Equal(t, "Case Statuses", structuredData[fieldName])

	statusValues := collectNames(t, structuredData, fieldData, fieldValue)

	require.Contains(t, statusValues, "New")
	require.Contains(t, statusValues, "InProgress")
}

func TestGetResourceTrailingSlashEquivalence(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	noSlashData := getResourceStructured(t, mcpClient, "metadata/automation")
	withSlashData := getResourceStructured(t, mcpClient, "hive://metadata/automation/")

	require.Equal(t, "hive://metadata/automation", noSlashData[argURI])
	require.Equal(t, "hive://metadata/automation", withSlashData[argURI])
	require.Equal(t, noSlashData[fieldSubcategories], withSlashData[fieldSubcategories])

	// Compare resources as sets: order may vary.
	noSlashResources, ok := noSlashData[fieldResources].([]any)
	require.True(t, ok)
	withSlashResources, ok := withSlashData[fieldResources].([]any)
	require.True(t, ok)

	require.Len(t, withSlashResources, len(noSlashResources), "should have same number of resources")

	noSlashResourcesMap := make(map[string]any)
	withSlashResourcesMap := make(map[string]any)

	for _, res := range noSlashResources {
		resMap, ok := res.(map[string]any)
		require.True(t, ok)

		name, ok := resMap[fieldName].(string)
		require.True(t, ok)

		noSlashResourcesMap[name] = resMap
	}

	for _, res := range withSlashResources {
		resMap, ok := res.(map[string]any)
		require.True(t, ok)

		name, ok := resMap[fieldName].(string)
		require.True(t, ok)

		withSlashResourcesMap[name] = resMap
	}

	require.Equal(t, noSlashResourcesMap, withSlashResourcesMap, "resources should be equivalent regardless of order")
}

func TestGetResourceResourcesFieldBehavior(t *testing.T) {
	testutils.Parallel(t)
	mcpClient := newResourceClient(t)

	// Browse a category that has only subcategories (no direct resources)
	structuredData := getResourceStructured(t, mcpClient, "metadata")

	subcategories, exists := structuredData[fieldSubcategories]
	require.True(t, exists, "subcategories field should exist")
	require.NotNil(t, subcategories, "subcategories should not be null")

	subcategoriesList, ok := subcategories.([]any)
	require.True(t, ok)
	require.NotEmpty(t, subcategoriesList, "Should contain automation, entities, organisation subcategories")
}
