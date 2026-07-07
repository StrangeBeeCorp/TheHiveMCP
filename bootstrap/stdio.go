package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func makeStdioAuthContextFunc(options *types.TheHiveMcpDefaultOptions) func(context.Context) context.Context {
	return func(ctx context.Context) context.Context {
		newCtx, err := AddTheHiveClientToContext(ctx)
		if err != nil {
			slog.Error("Failed to add TheHive client to context from environment variables", "error", err)
			return context.WithValue(ctx, types.AuthErrorCtxKey, fmt.Errorf("TheHive authentication failed: %w", err))
		}

		newCtx = validateTheHiveAuthInContext(newCtx, nil, nil)

		if options.DefaultCortexID != "" {
			newCtx = context.WithValue(newCtx, types.DefaultCortexIDCtxKey, options.DefaultCortexID)
		}

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
		return errors.New("MCP server cannot be nil")
	}

	if options == nil {
		return errors.New("options cannot be nil")
	}

	slog.Info("Starting STDIO server with context injection")

	contextFunc := makeStdioAuthContextFunc(options)

	err := server.ServeStdio(s, server.WithStdioContextFunc(contextFunc))
	if err != nil {
		slog.Error("Failed to start STDIO server", "error", err)
		return fmt.Errorf("failed to start STDIO server: %w", err)
	}

	return nil
}
