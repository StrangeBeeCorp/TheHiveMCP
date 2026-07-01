package testutils

import (
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// CallTool asserts the transport call succeeded with a non-nil result. It does
// not assert result.IsError, so it serves both success and error cases.
func CallTool(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	result, err := c.CallTool(t.Context(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	return result
}

// CallToolOK is like CallTool but additionally asserts the tool did not report an error.
func CallToolOK(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	result := CallTool(t, c, name, args)
	require.False(t, result.IsError, "tool %q should not report an error", name)

	return result
}

// StructuredData returns result.StructuredContent as a map[string]any, asserting the type.
func StructuredData(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()

	data, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok, "StructuredContent should be a map[string]any")

	return data
}

// RequirePermissionDenied asserts the tool reported an error whose text indicates
// the operation was not permitted.
func RequirePermissionDenied(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()

	require.True(t, result.IsError, "operation should be denied")
	require.NotEmpty(t, result.Content, "error result should carry content")
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "first content item should be text")
	require.Contains(t, text.Text, "not permitted")
}
