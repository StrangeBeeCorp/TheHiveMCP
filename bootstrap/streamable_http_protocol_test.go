package bootstrap

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// The integration suite drives the server over the in-process transport, so
// nothing in it observes Streamable HTTP — the transport every deployment
// actually serves. That gap is what hid the elicitation breakage on MCP
// 2026-07-28: in-process kept working while HTTP did not. These tests exercise
// the real transport configuration, built by StreamableHTTPOptions, so a
// protocol-level regression fails here rather than in production.
//
// None of them need a live TheHive: negotiation and listing happen before any
// credential is used.
func servedOverStreamableHTTP(t *testing.T) string {
	t.Helper()

	options := &types.TheHiveMcpDefaultOptions{
		TheHiveURL:            testDefaultHiveURL,
		TheHiveOrganisation:   testOrg,
		MCPServerEndpointPath: "/mcp",
	}
	handler := server.NewStreamableHTTPServer(GetMCPServerAndRegisterTools(), StreamableHTTPOptions(options)...)

	testServer := httptest.NewServer(handler)
	t.Cleanup(testServer.Close)

	return testServer.URL + options.MCPServerEndpointPath
}

func streamableHTTPClient(t *testing.T, endpoint string, opts ...client.ClientOption) *client.Client {
	t.Helper()

	clientTransport, err := transport.NewStreamableHTTP(endpoint)
	require.NoError(t, err)

	mcpClient := client.NewClient(clientTransport, opts...)
	require.NoError(t, mcpClient.Start(t.Context()))
	t.Cleanup(func() { _ = mcpClient.Close() })

	return mcpClient
}

func initializeOverHTTP(t *testing.T, mcpClient *client.Client, protocolVersion string) *mcp.InitializeResult {
	t.Helper()

	result, err := mcpClient.Initialize(t.Context(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: protocolVersion,
			ClientInfo:      mcp.Implementation{Name: "protocol probe", Version: "1.0.0"},
			Capabilities:    mcp.ClientCapabilities{},
		},
	})
	require.NoError(t, err)

	return result
}

// The fence: a client asking for the newest revision must be brought down to
// the handshake era rather than served 2026-07-28. Serving the modern protocol
// is a deliberate future change, and this test is what makes it deliberate —
// lifting the fence without replacing this assertion is the failure mode.
func TestStreamableHTTP_ModernClientNegotiatesDownToLegacy(t *testing.T) {
	t.Parallel()

	result := initializeOverHTTP(t, streamableHTTPClient(t, servedOverStreamableHTTP(t)), mcp.LATEST_PROTOCOL_VERSION)

	assert.False(t, mcp.IsModernProtocol(result.ProtocolVersion),
		"the transport is fenced to legacy revisions, so a modern client must negotiate down; got %q", result.ProtocolVersion)
	assert.Equal(t, mcp.LATEST_LEGACY_PROTOCOL_VERSION, result.ProtocolVersion,
		"negotiation should settle on the newest handshake revision both sides support")
}

// server/discover is the modern entry point. While fenced it must not answer,
// or a client would believe the stateless path is available.
func TestStreamableHTTP_ServerDiscoverIsNotServedWhileFenced(t *testing.T) {
	t.Parallel()

	body := `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{` +
		`"io.modelcontextprotocol/protocolVersion":"` + mcp.ProtocolVersion20260728 + `",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1.0.0"}}}}`

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, servedOverStreamableHTTP(t), strings.NewReader(body))
	require.NoError(t, err)

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Protocol-Version", mcp.ProtocolVersion20260728)
	request.Header.Set("Mcp-Method", "server/discover")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.NotContains(t, string(payload), `"supportedVersions"`,
		"a fenced server must not answer server/discover: %s", string(payload))
}

// A legacy client is what every deployment serves today, so the handshake path
// must keep working over the real transport, not only in-process.
func TestStreamableHTTP_LegacyClientCompletesHandshake(t *testing.T) {
	t.Parallel()

	mcpClient := streamableHTTPClient(t, servedOverStreamableHTTP(t),
		client.WithProtocolVersion(mcp.LATEST_LEGACY_PROTOCOL_VERSION))

	result := initializeOverHTTP(t, mcpClient, mcp.LATEST_LEGACY_PROTOCOL_VERSION)

	assert.Equal(t, mcp.LATEST_LEGACY_PROTOCOL_VERSION, result.ProtocolVersion)
	assert.NotNil(t, result.Capabilities.Tools, "tools must be advertised over Streamable HTTP")
	assert.NotNil(t, result.Capabilities.Resources, "resources must be advertised over Streamable HTTP")
	assert.Nil(t, result.Capabilities.Elicitation,
		"elicitation is no longer implemented, so it must not be advertised over this transport either")
}

// Tools and resources must actually list over Streamable HTTP. The in-process
// suite covers the handlers; this covers the wire.
func TestStreamableHTTP_ToolsAndResourcesList(t *testing.T) {
	t.Parallel()

	mcpClient := streamableHTTPClient(t, servedOverStreamableHTTP(t))
	initializeOverHTTP(t, mcpClient, mcp.LATEST_PROTOCOL_VERSION)

	tools, err := mcpClient.ListTools(t.Context(), mcp.ListToolsRequest{})
	require.NoError(t, err)

	toolNames := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	assert.ElementsMatch(t,
		[]string{"execute-automation", "get-resource", "manage-entities", "search-entities"},
		toolNames,
		"every registered tool must be reachable over Streamable HTTP")

	resources, err := mcpClient.ListResources(t.Context(), mcp.ListResourcesRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, resources.Resources, "the resource catalog must list over Streamable HTTP")
}
