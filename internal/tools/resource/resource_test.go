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
				"category": "schema",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify category response structure
	require.Equal(t, "schema", structuredData["category"])
	require.Equal(t, "hive://schema/", structuredData["uri"])

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
				"category": "metadata",
			},
		},
	}

	result, err := mcpClient.CallTool(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	structuredData, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)

	// Verify category response structure
	require.Equal(t, "metadata", structuredData["category"])
	require.Equal(t, "hive://metadata/", structuredData["uri"])

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
