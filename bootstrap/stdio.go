package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/server"
)

func makeStdioAuthContextFunc(options *types.TheHiveMcpDefaultOptions) func(context.Context) context.Context {
	return func(ctx context.Context) context.Context {
		// Add TheHive client to context from environment variables
		newCtx, err := AddTheHiveClientToContext(ctx)
		if err != nil {
			slog.Warn("Failed to add TheHive client to context from environment variables", "error", err)
			return ctx
		}

		// Add OpenAI client to context from environment variables
		newCtx, err = AddOpenAIClientToContext(newCtx)
		if err != nil {
			slog.Warn("Failed to add OpenAI client to context from environment variables", "error", err)
			// Don't return early here since OpenAI is optional
		}

		// Add permissions to context
		newCtx, err = AddPermissionsToContext(newCtx, options)
		if err != nil {
			slog.Warn("Failed to add permissions to context", "error", err)
			return ctx
		}

		return newCtx
	}
}

// StartStdioServer starts the STDIO server with production-ready configuration and error handling
func StartStdioServer(s *server.MCPServer, options *types.TheHiveMcpDefaultOptions) error {
	if s == nil {
		return fmt.Errorf("MCP server cannot be nil")
	}
	if options == nil {
		return fmt.Errorf("options cannot be nil")
	}

	slog.Info("Starting STDIO server with context injection")

	contextFunc := makeStdioAuthContextFunc(options)
	if err := server.ServeStdio(s, server.WithStdioContextFunc(contextFunc)); err != nil {
		slog.Error("Failed to start STDIO server", "error", err)
		return fmt.Errorf("failed to start STDIO server: %w", err)
	}

	return nil
}
