package resource_test

import (
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// TestGetResourceCatalog tests fetching the root catalog
func TestGetResourceCatalog(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "get-resource",
			Arguments: map[string]any{},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify catalog structure
	require.Equal(t, "hive://catalog", structuredData["uri"])
	require.Equal(t, "Resource Catalog", structuredData["name"])
	require.Equal(t, "application/json", structuredData["mimeType"])

	// Verify data contains categories
	data, ok := structuredData["data"].(map[string]any)
	require.True(t, ok)

	categories, ok := data["categories"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, categories)

	// Verify expected categories exist
	categoryNames := make([]string, 0)
	for _, cat := range categories {
		catMap := cat.(map[string]any)
		categoryNames = append(categoryNames, catMap["name"].(string))
	}

	require.Contains(t, categoryNames, "config")
	require.Contains(t, categoryNames, "schema")
	require.Contains(t, categoryNames, "metadata")
	require.Contains(t, categoryNames, "docs")
}

// TestGetResourceBrowseSchemaCategory tests browsing the schema category
func TestGetResourceBrowseSchemaCategory(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get-resource",
			Arguments: map[string]any{
				"uri": "schema",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify category response structure
	require.Equal(t, "hive://schema", structuredData["uri"])

	// Verify resources list
	resources, ok := structuredData["resources"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, resources)

	// Convert to string slice for easier checking
	resourceNames := make([]string, 0)
	for _, res := range resources {
		resMap := res.(map[string]any)
		resourceNames = append(resourceNames, resMap["name"].(string))
	}

	// Verify expected schemas are present
	require.Contains(t, resourceNames, "Alert Output Schema")
	require.Contains(t, resourceNames, "Case Output Schema")
	require.Contains(t, resourceNames, "Task Output Schema")
	require.Contains(t, resourceNames, "Observable Output Schema")
}

// TestGetResourceFetchAlertSchema tests fetching a specific static resource
func TestGetResourceFetchAlertSchema(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get-resource",
			Arguments: map[string]any{
				"uri": "hive://schema/alert",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify resource response structure
	require.Equal(t, "hive://schema/alert", structuredData["uri"])
	require.Equal(t, "Alert Output Schema", structuredData["name"])
	require.Equal(t, "application/json", structuredData["mimeType"])

	// Verify data contains schema information
	data, ok := structuredData["data"].(map[string]any)
	require.True(t, ok)

	// Check for expected alert fields in schema
	fields, ok := data["properties"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, fields)

	// Verify some key alert fields exist
	fieldNames := make([]string, 0)
	for fieldName := range fields {
		fieldNames = append(fieldNames, fieldName)
	}

	require.Contains(t, fieldNames, "title")
	require.Contains(t, fieldNames, "severity")
	require.Contains(t, fieldNames, "type")
	require.Contains(t, fieldNames, "source")
}

// TestGetResourceFetchCurrentUser tests fetching a dynamic resource
func TestGetResourceFetchCurrentUser(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get-resource",
			Arguments: map[string]any{
				"uri": "hive://config/current-user",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify resource response structure
	require.Equal(t, "hive://config/current-user", structuredData["uri"])
	require.Equal(t, "Current User", structuredData["name"])
	require.Equal(t, "application/json", structuredData["mimeType"])

	// Verify data contains user information
	data, ok := structuredData["data"].(map[string]any)
	require.True(t, ok)

	// Check for expected user fields
	require.Contains(t, data, "login")
	require.Contains(t, data, "name")
	require.NotEmpty(t, data["login"])

	// Verify it's the admin user from test setup
	require.Equal(t, "admin@thehive.local", data["login"])
}

// TestGetResourceFetchDocumentation tests fetching documentation resources
func TestGetResourceFetchDocumentation(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get-resource",
			Arguments: map[string]any{
				"uri": "hive://docs/entities/case",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify resource response structure
	require.Equal(t, "hive://docs/entities/case", structuredData["uri"])
	require.Equal(t, "Case Documentation", structuredData["name"])
	require.Equal(t, "text/plain", structuredData["mimeType"])

	// Verify data contains documentation
	data, ok := structuredData["content"].(string)
	require.True(t, ok)
	require.NotEmpty(t, data)

	// Check for expected content in documentation
	require.Contains(t, data, "A case is a structured entity used to track, investigate,")
}

// TestGetResourceBrowseMetadataCategory tests browsing metadata with subcategories
func TestGetResourceBrowseMetadataCategory(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get-resource",
			Arguments: map[string]any{
				"uri": "metadata",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify category response structure
	require.Equal(t, "hive://metadata", structuredData["uri"])

	// Verify subcategories exist for metadata
	subcategories, ok := structuredData["subcategories"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, subcategories)

	// Verify expected subcategories
	subcategoryNames := make([]string, 0)
	for _, subcat := range subcategories {
		subcatMap := subcat.(map[string]any)
		subcategoryNames = append(subcategoryNames, subcatMap["name"].(string))
	}

	require.Contains(t, subcategoryNames, "entities")
	require.Contains(t, subcategoryNames, "automation")
	require.Contains(t, subcategoryNames, "organization")
}

// TestGetResourceFetchCaseStatuses tests fetching case statuses metadata
func TestGetResourceFetchCaseStatuses(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get-resource",
			Arguments: map[string]any{
				"uri": "hive://metadata/entities/case/statuses",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify resource response structure
	require.Equal(t, "hive://metadata/entities/case/statuses", structuredData["uri"])
	require.Equal(t, "Case Statuses", structuredData["name"])

	// Verify data contains statuses
	statuses, ok := structuredData["data"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, statuses)

	// Verify expected statuses exist
	statusValues := make([]string, 0)
	for _, status := range statuses {
		statusMap := status.(map[string]any)
		statusValues = append(statusValues, statusMap["value"].(string))
	}

	require.Contains(t, statusValues, "New")
	require.Contains(t, statusValues, "InProgress")
}

// TestGetResourceTrailingSlashEquivalence tests that URIs work with or without trailing slashes
func TestGetResourceTrailingSlashEquivalence(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	// Get results using URI without trailing slash
	noSlashRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get-resource",
			Arguments: map[string]any{
				"uri": "metadata/automation",
			},
		},
	}

	noSlashResult, err := mcpClient.CallTool(t.Context(), noSlashRequest)
	require.NoError(t, err, "URI without trailing slash should work")
	require.NotNil(t, noSlashResult)

	noSlashData, ok := noSlashResult.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Get results using URI with trailing slash
	withSlashRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get-resource",
			Arguments: map[string]any{
				"uri": "hive://metadata/automation/",
			},
		},
	}

	withSlashResult, err := mcpClient.CallTool(t.Context(), withSlashRequest)
	require.NoError(t, err, "URI with trailing slash should work")
	require.NotNil(t, withSlashResult)

	withSlashData, ok := withSlashResult.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify both approaches return equivalent results
	require.Equal(t, "hive://metadata/automation", noSlashData["uri"])
	require.Equal(t, "hive://metadata/automation", withSlashData["uri"])
	require.Equal(t, noSlashData["subcategories"], withSlashData["subcategories"])

	// For resources, we need to compare sets as order may vary
	noSlashResources, ok := noSlashData["resources"].([]any)
	require.True(t, ok)
	withSlashResources, ok := withSlashData["resources"].([]any)
	require.True(t, ok)

	require.Equal(t, len(noSlashResources), len(withSlashResources), "should have same number of resources")

	// Convert to maps for easier comparison
	noSlashResourcesMap := make(map[string]any)
	withSlashResourcesMap := make(map[string]any)

	for _, res := range noSlashResources {
		resMap := res.(map[string]any)
		noSlashResourcesMap[resMap["name"].(string)] = resMap
	}

	for _, res := range withSlashResources {
		resMap := res.(map[string]any)
		withSlashResourcesMap[resMap["name"].(string)] = resMap
	}

	require.Equal(t, noSlashResourcesMap, withSlashResourcesMap, "resources should be equivalent regardless of order")
}

// TestGetResourceResourcesFieldBehavior tests the resources field behavior in category responses
func TestGetResourceResourcesFieldBehavior(t *testing.T) {
	testutils.SetupTestWithCleanup(t)
	mcpClient := testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)

	// Browse a category that has only subcategories (no direct resources)
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get-resource",
			Arguments: map[string]any{
				"uri": "metadata",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify subcategories field behavior
	subcategories, exists := structuredData["subcategories"]
	require.True(t, exists, "subcategories field should exist")
	require.NotNil(t, subcategories, "subcategories should not be null")

	subcategoriesList, ok := subcategories.([]any)
	require.True(t, ok)
	require.NotEmpty(t, subcategoriesList, "Should contain automation, entities, organization subcategories")

	// Check resources field behavior when no direct resources exist
	resources, exists := structuredData["resources"]
	require.True(t, exists, "resources field should exist")

	if resources == nil {
		t.Log("resources field is null when no direct resources exist")
	} else {
		resourcesList, ok := resources.([]any)
		require.True(t, ok)
		require.Empty(t, resourcesList, "Should be empty array when no direct resources")
	}
}
