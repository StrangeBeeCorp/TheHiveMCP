package bootstrap

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// advertisedCapabilities returns what a client actually sees, via a real
// initialize over the in-process transport. Needs no TheHive instance.
func advertisedCapabilities(t *testing.T, mcpServer *server.MCPServer) mcp.ServerCapabilities {
	t.Helper()

	mcpClient := client.NewClient(transport.NewInProcessTransport(mcpServer))

	require.NoError(t, mcpClient.Start(t.Context()))

	t.Cleanup(func() { _ = mcpClient.Close() })

	result, err := mcpClient.Initialize(t.Context(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "capability probe", Version: "1.0.0"},
			Capabilities:    mcp.ClientCapabilities{},
		},
	})
	require.NoError(t, err)

	return result.Capabilities
}

func testServers(t *testing.T) map[string]*server.MCPServer {
	t.Helper()

	return map[string]*server.MCPServer{
		"served": GetMCPServer(),
		"in-process": GetInprocessServer(
			&TheHiveCredentials{URL: "https://thehive.example.com", APIKey: "probe-key"}, ""),
	}
}

// A capability advertised but not implemented is worse than one omitted: the
// client believes the request is available, issues it, and gets an error.
func TestResourceCapabilities_DoNotAdvertiseSubscriptions(t *testing.T) {
	t.Parallel()

	for name, mcpServer := range testServers(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resources := advertisedCapabilities(t, mcpServer).Resources
			require.NotNil(t, resources, "resources capability must be present; the server serves resources")

			assert.False(t, resources.Subscribe,
				"resources/subscribe has no handler and no change events to send, so it must not be advertised")
			assert.True(t, resources.ListChanged,
				"listChanged is trivially satisfied by a resource list fixed at startup")
		})
	}
}

// Nothing registers a prompt, so the capability must not be declared.
func TestPromptCapabilities_AreNotAdvertised(t *testing.T) {
	t.Parallel()

	for name, mcpServer := range testServers(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, advertisedCapabilities(t, mcpServer).Prompts,
				"no prompts are registered, so the capability must not be advertised")
		})
	}
}

// The confirmation layer was removed with the mcp-go v1.0.0 upgrade: MCP
// 2026-07-28 has no server-initiated requests, and elicitation saw almost no
// client adoption (ADR-0003, #170). Authorisation is the caller's API key plus
// the permissions config, so nothing about access changed — but the capability
// must stop being advertised along with the implementation, or clients are told
// to expect prompts that will never arrive.
func TestElicitationCapability_IsNotAdvertised(t *testing.T) {
	t.Parallel()

	for name, mcpServer := range testServers(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, advertisedCapabilities(t, mcpServer).Elicitation,
				"elicitation is no longer implemented, so the capability must not be advertised")
		})
	}
}

// Guards why dropping it is safe: AddPrompt registers the capability itself, so
// adding prompts later cannot leave it undeclared.
func TestPromptCapabilities_ReappearWhenAPromptIsRegistered(t *testing.T) {
	t.Parallel()

	mcpServer := GetMCPServer()
	mcpServer.AddPrompt(
		mcp.Prompt{Name: "probe", Description: "registered only by this test"},
		func(context.Context, mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{}, nil
		},
	)

	assert.NotNil(t, advertisedCapabilities(t, mcpServer).Prompts,
		"registering a prompt must advertise the capability without anyone remembering to")
}

// If the two constructors drift, integration tests exercise a capability set no
// deployment serves — a difference that only surfaces in production.
func TestServerConstructors_AdvertiseTheSameCapabilities(t *testing.T) {
	t.Parallel()

	servers := testServers(t)

	assert.Equal(t,
		advertisedCapabilities(t, servers["served"]),
		advertisedCapabilities(t, servers["in-process"]))
}
