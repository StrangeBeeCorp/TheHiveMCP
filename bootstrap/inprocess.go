package bootstrap

import (
	"context"
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/version"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func AuthMiddleware(creds *TheHiveCredentials) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			newCtx, err := AddTheHiveClientToContextWithCreds(ctx, creds)
			if err != nil {
				return nil, fmt.Errorf("failed to add TheHive client to context: %w", err)
			}
			return next(newCtx, request)
		}
	}
}

func GetInprocessServer(creds *TheHiveCredentials) *server.MCPServer {
	mcpServer := server.NewMCPServer(
		"TheHiveMCP",
		version.GetVersion(),
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithHooks(logging.GetLoggingHooks()),
		server.WithElicitation(),
		server.WithToolHandlerMiddleware(AuthMiddleware(creds)),
	)
	mcpServer.EnableSampling()
	return mcpServer
}
