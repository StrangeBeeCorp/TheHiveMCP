package bootstrap

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// callTool calls a tool through the in-process server, against the TheHive at url, with the
// permission profile at permissions ("" is the default one).
func callTool(t *testing.T, url, permissions, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	mcpServer := GetInprocessServer(&TheHiveCredentials{URL: url, APIKey: "stub-key", Organisation: "stub-org"}, permissions)
	RegisterToolsToMCPServer(mcpServer)

	c := client.NewClient(transport.NewInProcessTransport(mcpServer))
	require.NoError(t, c.Start(t.Context()))

	_, err := c.Initialize(t.Context(), mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, ClientInfo: mcp.Implementation{Name: "stub-client", Version: "1"},
	}})
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Name = name
	request.Params.Arguments = args

	result, err := c.CallTool(t.Context(), request)
	require.NoError(t, err)

	return result
}

// resultText is the text a tool result carries.
func resultText(result *mcp.CallToolResult) string {
	var b strings.Builder

	for _, c := range result.Content {
		if text, ok := c.(mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}

	return b.String()
}
