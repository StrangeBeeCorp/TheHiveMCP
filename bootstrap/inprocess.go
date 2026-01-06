package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/version"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func AuthMiddleware(creds *TheHiveCredentials, openaiCreds *OpenAICredentials, permissionsConfigPath string) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Add TheHive client to context
			newCtx, err := AddTheHiveClientToContextWithCreds(ctx, creds)
			if err != nil {
				return nil, fmt.Errorf("failed to add TheHive client to context: %w", err)
			}

			// Add OpenAI client to context if credentials provided
			if openaiCreds != nil {
				newCtx, err = AddOpenAIClientToContextWithCreds(newCtx, openaiCreds)
				if err != nil {
					slog.Warn("Failed to add OpenAI client to context", "error", err)
					// Don't fail since OpenAI is optional
				}
			}

			// Add permissions to context
			permsConfig, err := LoadPermissions(permissionsConfigPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load permissions: %w", err)
			}
			newCtx = context.WithValue(newCtx, types.PermissionsCtxKey, permsConfig)

			return next(newCtx, request)
		}
	}
}

func GetInprocessServer(creds *TheHiveCredentials, openaiCreds *OpenAICredentials, permissionsConfigPath string) *server.MCPServer {
	mcpServer := server.NewMCPServer(
		"TheHiveMCP",
		version.GetVersion(),
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithHooks(logging.GetLoggingHooks()),
		server.WithElicitation(),
		server.WithToolHandlerMiddleware(AuthMiddleware(creds, openaiCreds, permissionsConfigPath)),
	)
	mcpServer.EnableSampling()
	return mcpServer
}
