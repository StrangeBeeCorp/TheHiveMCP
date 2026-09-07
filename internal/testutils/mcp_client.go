package testutils

import (
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

// GetMCPTestClient creates an in-process MCP test client with admin permissions.
func GetMCPTestClient(t *testing.T) *client.Client {
	t.Helper()

	return GetMCPTestClientWithPermissions(t, string(types.PermissionConfigAdmin))
}

// GetMCPTestClientWithPermissions creates a test client. permissionsConfigPath:
// - types.PermissionConfigAdmin — admin
// - types.PermissionConfigReadOnly — read-only
// - testutils.PermissionsFixture(t, "analyst.yaml") — file path
// - "" — default read-only
func GetMCPTestClientWithPermissions(
	t *testing.T,
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

	client := client.NewClient(transport.NewInProcessTransport(mcpServer))

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
				Capabilities: mcp.ClientCapabilities{},
			},
		},
	)
	if err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}

	return client
}
