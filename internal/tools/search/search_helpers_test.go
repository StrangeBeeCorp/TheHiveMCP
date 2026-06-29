package search_test

import (
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// newSearchClient returns a test MCP client wired with the no-op sampling
// handler (search-entities must never sample) and the default accept-elicitation
// handler, using admin permissions.
func newSearchClient(t *testing.T) *client.Client {
	t.Helper()
	return testutils.GetMCPTestClient(t, unusedSamplingHandler(t), testutils.DummyElicitationAccept)
}

// callSearch invokes the search-entities tool with the given arguments and
// returns the raw tool result, asserting the transport call itself succeeded.
func callSearch(t *testing.T, c *client.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := c.CallTool(t.Context(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "search-entities", Arguments: args},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

// searchStructured invokes search-entities, asserts the tool did not return an
// error result, and returns the structured content map.
func searchStructured(t *testing.T, c *client.Client, args map[string]any) map[string]any {
	t.Helper()
	result := callSearch(t, c, args)
	require.False(t, result.IsError, "unexpected tool error: %s", resultText(t, result))
	data, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	return data
}

// searchRows invokes search-entities and returns the "results" rows.
func searchRows(t *testing.T, c *client.Client, args map[string]any) []any {
	t.Helper()
	data := searchStructured(t, c, args)
	rows, ok := data["results"].([]any)
	require.True(t, ok)
	return rows
}
