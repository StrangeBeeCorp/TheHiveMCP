package bootstrap

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/version"
)

// AuthMiddleware returns a tool-handler middleware for the in-process server
// that injects a TheHive client built from the given trusted credentials, marks
// authentication as validated, and loads permissions from the given config path.
func AuthMiddleware(creds *TheHiveCredentials, permissionsConfigPath string) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			newCtx, err := AddTheHiveClientToContextWithCreds(ctx, creds)
			if err != nil {
				return nil, fmt.Errorf("failed to add TheHive client to context: %w", err)
			}
			// In-process creds come from the trusted host app; mark auth validated.
			newCtx = context.WithValue(newCtx, types.AuthValidatedCtxKey, true)

			permsConfig, err := LoadPermissions(permissionsConfigPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load permissions: %w", err)
			}

			newCtx = context.WithValue(newCtx, types.PermissionsCtxKey, permsConfig)

			return next(newCtx, request)
		}
	}
}

// GetInprocessServer builds an MCP server for in-process use, wired with the
// in-process AuthMiddleware for the given credentials and permissions config.
func GetInprocessServer(creds *TheHiveCredentials, permissionsConfigPath string) *server.MCPServer {
	mcpServer := server.NewMCPServer(
		"TheHiveMCP",
		version.GetVersion(),
		server.WithToolCapabilities(true),
		// Must mirror GetMCPServer, or integration tests exercise a capability set
		// no deployment serves.
		server.WithResourceCapabilities(false, true),
		server.WithHooks(logging.GetLoggingHooks()),
		server.WithToolHandlerMiddleware(AuthMiddleware(creds, permissionsConfigPath)),
		// Deliberately stricter than GetMCPServer, and the one intended
		// divergence from it: this validates each result against the tool's own
		// declared output schema and turns a violation into an error result, so
		// the integration suite fails on a non-conforming payload instead of
		// accepting it the way mcp-go's client does.
		//
		// It belongs here rather than in a test helper because this is the real
		// chokepoint — tests call tools both through testutils.CallTool and
		// directly on the client, and only the server sees every call.
		//
		// Not enabled in production: a deployment that starts violating its own
		// schema should degrade, not have every call replaced by an error.
		server.WithOutputSchemaValidation(),
	)

	return mcpServer
}
