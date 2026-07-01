package search_test

import (
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// Admin client with a sampling handler that fails if hit: search-entities must never sample.
func newSearchClient(t *testing.T) *client.Client {
	t.Helper()
	return testutils.GetMCPTestClient(t, unusedSamplingHandler(t), testutils.DummyElicitationAccept)
}

func callSearch(t *testing.T, c *client.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	return testutils.CallTool(t, c, "search-entities", args)
}

func searchStructured(t *testing.T, c *client.Client, args map[string]any) map[string]any {
	t.Helper()
	result := testutils.CallToolOK(t, c, "search-entities", args)
	return testutils.StructuredData(t, result)
}

func searchRows(t *testing.T, c *client.Client, args map[string]any) []any {
	t.Helper()
	data := searchStructured(t, c, args)
	rows, ok := data["results"].([]any)
	require.True(t, ok)
	return rows
}
