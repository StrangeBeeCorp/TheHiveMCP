package testutils

import (
	"context"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/bootstrap"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// initTestLoggerOnce guards slog.SetDefault against a data race, since tests
// run concurrently.
var initTestLoggerOnce sync.Once

func initTestLogger(options *types.TheHiveMcpDefaultOptions) {
	initTestLoggerOnce.Do(func() {
		logging.InitLogger(options.LogLevel, options.TransportType)
	})
}

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

// SamplingHandlerCreateMessageFromStringResponse returns a sampling handler that
// always replies with the given response text.
func SamplingHandlerCreateMessageFromStringResponse(response string) func(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	return func(_ context.Context, _ mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
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

// DummyElicitationAccept is an elicitation handler that always accepts with a
// fixed mock payload, for tests that only need elicitation to succeed.
func DummyElicitationAccept(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
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

// DummySamplingHandlerCreateMessage is a sampling handler that replies with a
// fixed dummy response, for tests that only need sampling to succeed.
func DummySamplingHandlerCreateMessage(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	return SamplingHandlerCreateMessageFromStringResponse("This is a dummy response")(ctx, request)
}

// GetMCPTestClient creates an in-process MCP test client with admin permissions
// and the given sampling and elicitation handlers.
func GetMCPTestClient(
	t *testing.T,
	samplingHandlerCreateMessage func(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error),
	elicitationHandlerElicit func(ctx context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error),
) *client.Client {
	t.Helper()

	return GetMCPTestClientWithPermissions(t, samplingHandlerCreateMessage, elicitationHandlerElicit, string(types.PermissionConfigAdmin))
}

// GetMCPTestClientWithPermissions creates a test client. permissionsConfigPath:
// - types.PermissionConfigAdmin — admin
// - types.PermissionConfigReadOnly — read-only
// - testutils.PermissionsFixture(t, "analyst.yaml") — file path
// - "" — default read-only
func GetMCPTestClientWithPermissions(
	t *testing.T,
	samplingHandlerCreateMessage func(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error),
	elicitationHandlerElicit func(ctx context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error),
	permissionsConfigPath string,
) *client.Client {
	t.Helper()

	containerURL, err := StartTheHiveContainer(t)
	if err != nil {
		t.Fatalf("Failed to get container URL: %v", err)
	}

	options := NewMCPTestConfig()
	initTestLogger(options)

	env := testEnvFor(t)
	creds := &bootstrap.TheHiveCredentials{
		URL:          containerURL,
		APIKey:       options.TheHiveAPIKey,
		Username:     env.username,
		Password:     env.password,
		Organisation: env.org,
	}
	mcpServer := bootstrap.GetInprocessServer(creds, permissionsConfigPath)
	bootstrap.RegisterToolsToMCPServer(mcpServer)

	serverSamplingHandler := &functionBasedSamplingHandler{createMessageFunc: samplingHandlerCreateMessage}
	serverElicitationHandler := &functionBasedElicitationHandler{elicitFunc: elicitationHandlerElicit}

	inProcessTransport := transport.NewInProcessTransportWithOptions(mcpServer,
		transport.WithSamplingHandler(serverSamplingHandler),
		transport.WithElicitationHandler(serverElicitationHandler),
	)

	client := client.NewClient(inProcessTransport)

	err = client.Start(t.Context())
	if err != nil {
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
