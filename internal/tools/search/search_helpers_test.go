package search_test

import (
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
)

// Admin client. Sampling is no longer wired up at all — the client advertises
// no such capability — so search-entities could not reach an LLM if it tried.
func newSearchClient(t *testing.T) *client.Client {
	t.Helper()
	return testutils.GetMCPTestClient(t)
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
