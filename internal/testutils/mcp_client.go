package testutils

import (
	"context"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/bootstrap"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

type functionBasedSamplingHandler struct {
	createMessageFunc func(context.Context, mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error)
}

func (h *functionBasedSamplingHandler) CreateMessage(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	return h.createMessageFunc(ctx, request)
}

type functionBasedElicitationHandler struct {
	elicitFunc func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error)
}

func (h *functionBasedElicitationHandler) Elicit(ctx context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	return h.elicitFunc(ctx, request)
}

func SamplingHandlerCreateMessageFromStringResponse(response string) func(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	return func(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
		samplingMessage := mcp.SamplingMessage{
			Role: mcp.RoleAssistant,
			Content: mcp.TextContent{
				Type: "text",
				Text: response,
			},
		}

		return &mcp.CreateMessageResult{
			SamplingMessage: samplingMessage,
			Model:           "test-model",
			StopReason:      "endTurn",
		}, nil
	}
}

func DummyElicitationAccept(ctx context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	return &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action: mcp.ElicitationResponseActionAccept,
			Content: map[string]any{
				"confirmed": true,
				"details":   "Mock data provided by client",
			},
		},
	}, nil
}

func DummySamplingHandlerCreateMessage(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	return SamplingHandlerCreateMessageFromStringResponse("This is a dummy response")(ctx, request)
}

func GetMCPTestClient(
	t *testing.T,
	samplingHandlerCreateMessage func(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error),
	elicitationHandlerElicit func(ctx context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error),
) *client.Client {
	return GetMCPTestClientWithPermissions(t, samplingHandlerCreateMessage, elicitationHandlerElicit, string(types.PermissionConfigAdmin))
}

// GetMCPTestClientWithPermissions creates a test client with specific permissions configuration
// permissionsConfigPath can be:
// - types.PermissionConfigAdmin for admin permissions
// - types.PermissionConfigReadOnly for read-only permissions
// - "docs/examples/permissions/analyst.yaml" for analyst permissions (file path)
// - "" for default read-only permissions (empty string)
func GetMCPTestClientWithPermissions(
	t *testing.T,
	samplingHandlerCreateMessage func(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error),
	elicitationHandlerElicit func(ctx context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error),
	permissionsConfigPath string,
) *client.Client {
	t.Helper()

	// Get the actual container URL to use for MCP server
	containerURL, err := StartTheHiveContainer(t)
	if err != nil {
		t.Fatalf("Failed to get container URL: %v", err)
	}

	options := NewMCPTestConfig()
	creds := &bootstrap.TheHiveCredentials{
		URL:          containerURL, // Use actual container URL instead of hardcoded one
		APIKey:       options.TheHiveAPIKey,
		Username:     options.TheHiveUsername,
		Password:     options.TheHivePassword,
		Organisation: options.TheHiveOrganisation,
	}
	mcpServer := bootstrap.GetInprocessServer(creds, permissionsConfigPath)
	bootstrap.RegisterToolsToMCPServer(mcpServer)

	// Create wrappers that implement server.SamplingHandler and server.ElicitationHandler
	serverSamplingHandler := &functionBasedSamplingHandler{createMessageFunc: samplingHandlerCreateMessage}
	serverElicitationHandler := &functionBasedElicitationHandler{elicitFunc: elicitationHandlerElicit}

	inProcessTransport := transport.NewInProcessTransportWithOptions(mcpServer,
		transport.WithSamplingHandler(serverSamplingHandler),
		transport.WithElicitationHandler(serverElicitationHandler),
	)
	client := client.NewClient(inProcessTransport)
	if err := client.Start(t.Context()); err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}
	_, err = client.Initialize(
		t.Context(),
		mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo: mcp.Implementation{
					Name:    "MCP Test Client",
					Version: "1.0.0",
				},
				Capabilities: mcp.ClientCapabilities{
					Sampling:    &struct{}{},
					Elicitation: &struct{}{},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}
	return client
}
