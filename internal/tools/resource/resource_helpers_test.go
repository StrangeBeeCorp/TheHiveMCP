package resource_test

import (
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
)

// Tool name, argument keys, structured-content field names and MIME types
// repeated across the resource test bodies. Centralized to satisfy goconst and
// keep the assertions consistent.
const (
	toolNameGetResource = "get-resource"

	argURI = "uri"

	fieldName          = "name"
	fieldMimeType      = "mimeType"
	fieldData          = "data"
	fieldCategories    = "categories"
	fieldResources     = "resources"
	fieldSubcategories = "subcategories"
	fieldValue         = "value"

	mimeJSON = "application/json"
)

// newResourceClient builds an MCP client for the resource tests, wiring the
// standard setup + cleanup.
func newResourceClient(t *testing.T) *client.Client {
	t.Helper()
	testutils.SetupTestWithCleanup(t)

	return testutils.GetMCPTestClient(t, nil, testutils.DummyElicitationAccept)
}

// getResourceStructured calls get-resource with the given URI (empty for the
// catalog), asserts success, and returns the structured content as a map.
func getResourceStructured(t *testing.T, c *client.Client, uri string) map[string]any {
	t.Helper()

	args := map[string]any{}
	if uri != "" {
		args[argURI] = uri
	}

	result := testutils.CallToolOK(t, c, toolNameGetResource, args)

	return testutils.StructuredData(t, result)
}

// collectNames extracts the string value at key from every map in data[field],
// asserting the shapes along the way. Used to gather category/resource/
// subcategory/status names before asserting membership.
func collectNames(t *testing.T, data map[string]any, field, key string) []string {
	t.Helper()

	items, ok := data[field].([]any)
	require.True(t, ok)
	require.NotEmpty(t, items)

	names := make([]string, 0, len(items))

	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		require.True(t, ok)

		value, ok := itemMap[key].(string)
		require.True(t, ok)

		names = append(names, value)
	}

	return names
}

// requireResourceMeta asserts the uri/name/mimeType triple on a fetched resource.
func requireResourceMeta(t *testing.T, data map[string]any, uri, name, mimeType string) {
	t.Helper()

	require.Equal(t, uri, data[argURI])
	require.Equal(t, name, data[fieldName])
	require.Equal(t, mimeType, data[fieldMimeType])
}
