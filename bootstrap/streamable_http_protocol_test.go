package bootstrap

import (
	"encoding/json"
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
// servedOverStreamableHTTP serves the real tool set over the production
// transport configuration, including the legacy fence.
//
// It points at a local fakeTheHive rather than a public hostname. No credential
// reaches the auth context func in these tests, so validation fails before any
// outbound request is attempted and nothing is sent anywhere today — but the
// fixture should stay inert by construction, not by that coincidence.
func servedOverStreamableHTTP(t *testing.T) string {
	t.Helper()

	return serveMCP(t, StreamableHTTPOptions(protocolTestOptions(t)))
}

// servedUnfencedOverStreamableHTTP is the same server with the legacy fence
// removed, which is what lifting it in production will look like.
//
// Without this, every fixture negotiates down and the 2026-07-28 stateless path
// ships untested — the fence would be the only thing standing between us and an
// unexercised protocol. These tests are the evidence that removing it is safe.
func servedUnfencedOverStreamableHTTP(t *testing.T) string {
	t.Helper()

	options := protocolTestOptions(t)

	unfenced := []server.StreamableHTTPOption{
		server.WithEndpointPath(options.MCPServerEndpointPath),
		server.WithStateLess(false),
		server.WithHTTPContextFunc(GetHTTPAuthContextFunc(options)),
	}

	return serveMCP(t, unfenced)
}

func protocolTestOptions(t *testing.T) *types.TheHiveMcpDefaultOptions {
	t.Helper()

	return &types.TheHiveMcpDefaultOptions{
		TheHiveURL:            newFakeTheHive(t).url(),
		TheHiveOrganisation:   testOrg,
		MCPServerEndpointPath: "/mcp",
	}
}

func serveMCP(t *testing.T, options []server.StreamableHTTPOption) string {
	t.Helper()

	testServer := httptest.NewServer(server.NewStreamableHTTPServer(GetMCPServerAndRegisterTools(), options...))
	t.Cleanup(testServer.Close)

	return testServer.URL + "/mcp"
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

// discoverOverHTTP issues a raw server/discover, which cannot go through the
// client library: the library refuses to send it once negotiation has settled
// on a legacy revision, and refusing it is exactly what we assert here.
func discoverOverHTTP(t *testing.T, endpoint string) map[string]any {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{` +
		`"io.modelcontextprotocol/protocolVersion":"` + mcp.ProtocolVersion20260728 + `",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1.0.0"}}}}`

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(body))
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

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded), "undecodable discover response: %s", payload)

	return decoded
}

// While fenced, server/discover must be refused with the specific
// "unsupported protocol version" code. Asserting only that the response lacks
// a discovery payload would also pass on an empty body, a malformed reply, or
// an unrelated auth failure — none of which prove the fence did the refusing.
func TestStreamableHTTP_ServerDiscoverIsRefusedWhileFenced(t *testing.T) {
	t.Parallel()

	decoded := discoverOverHTTP(t, servedOverStreamableHTTP(t))

	rpcError, ok := decoded["error"].(map[string]any)
	require.True(t, ok, "a fenced server must answer server/discover with an error, got: %v", decoded)

	code, ok := rpcError["code"].(float64)
	require.True(t, ok, "error has no numeric code: %v", rpcError)

	assert.Equal(t, mcp.UNSUPPORTED_PROTOCOL_VERSION, int(code),
		"the refusal must be an unsupported-protocol-version error, got: %v", rpcError)
	assert.NotContains(t, decoded, "result", "a fenced server must not return a discovery result")
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
		[]string{toolExecuteAutomation, toolGetResource, toolManageEntities, toolSearchEntities},
		toolNames,
		"every registered tool must be reachable over Streamable HTTP")

	resources, err := mcpClient.ListResources(t.Context(), mcp.ListResourcesRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, resources.Resources, "the resource catalog must list over Streamable HTTP")
}

// Everything above runs against the fenced transport, so it only ever proves
// that modern clients are turned away. These two exercise the unfenced
// configuration — the 2026-07-28 stateless path ADR-0003 defers rather than
// abandons — so that lifting the fence is backed by evidence instead of hope.

// With the fence removed the server must actually negotiate the modern
// revision, not merely stop refusing it.
func TestStreamableHTTP_UnfencedServerNegotiatesModern(t *testing.T) {
	t.Parallel()

	endpoint := servedUnfencedOverStreamableHTTP(t)

	result := initializeOverHTTP(t, streamableHTTPClient(t, endpoint), mcp.LATEST_PROTOCOL_VERSION)
	assert.Equal(t, mcp.ProtocolVersion20260728, result.ProtocolVersion,
		"an unfenced server must negotiate the modern revision")

	decoded := discoverOverHTTP(t, endpoint)

	discovery, ok := decoded["result"].(map[string]any)
	require.True(t, ok, "an unfenced server must answer server/discover, got: %v", decoded)

	versions, ok := discovery["supportedVersions"].([]any)
	require.True(t, ok, "discovery result has no supportedVersions: %v", discovery)
	assert.Contains(t, versions, mcp.ProtocolVersion20260728,
		"discovery must advertise the modern revision")
}

// Tools must be reachable over the stateless path, not just listed by it: a
// call is what a modern client actually does, and it is the step that broke
// under elicitation before that layer was removed.
func TestStreamableHTTP_UnfencedServerServesToolsStatelessly(t *testing.T) {
	t.Parallel()

	mcpClient := streamableHTTPClient(t, servedUnfencedOverStreamableHTTP(t))

	result := initializeOverHTTP(t, mcpClient, mcp.LATEST_PROTOCOL_VERSION)
	require.Equal(t, mcp.ProtocolVersion20260728, result.ProtocolVersion)

	tools, err := mcpClient.ListTools(t.Context(), mcp.ListToolsRequest{})
	require.NoError(t, err)

	toolNames := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	assert.ElementsMatch(t,
		[]string{toolExecuteAutomation, toolGetResource, toolManageEntities, toolSearchEntities},
		toolNames, "every tool must be reachable over the modern stateless path")

	for _, tool := range tools.Tools {
		if tool.Name == toolManageEntities {
			assert.NotEmpty(t, tool.InputSchema.Properties,
				"tools listed over the modern path must still advertise their parameters")
		}
	}

	// No credentials are configured here, so the call is rejected by the auth
	// middleware. That rejection is the assertion: reaching it means the
	// request was negotiated, routed and dispatched over the stateless path.
	// A protocol-level failure would instead complain about the version, the
	// method or the Mcp-Method header, and never mention credentials.
	_, err = mcpClient.CallTool(t.Context(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolSearchEntities,
			Arguments: map[string]any{"entity-type": "case"},
		},
	})
	require.Error(t, err, "the call must be rejected: these tests configure no credentials")
	assert.Contains(t, err.Error(), "TheHive authentication failed",
		"the modern call must fail on credentials, having already cleared the protocol layer")
}
