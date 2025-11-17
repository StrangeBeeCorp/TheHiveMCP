package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
)

func stdioAuthContextFunc(ctx context.Context) context.Context {
	// Use common function to add TheHive client to context from environment variables
	newCtx, err := AddTheHiveClientToContext(ctx)
	if err != nil {
		slog.Warn("Failed to add TheHive client to context from environment variables", "error", err)
		return ctx
	}

	return newCtx
}

// StartStdioServer starts the STDIO server with production-ready configuration and error handling
func StartStdioServer(s *server.MCPServer) error {
	if s == nil {
		return fmt.Errorf("MCP server cannot be nil")
	}

	slog.Info("Starting STDIO server with context injection")

	if err := server.ServeStdio(s, server.WithStdioContextFunc(stdioAuthContextFunc)); err != nil {
		slog.Error("Failed to start STDIO server", "error", err)
		return fmt.Errorf("failed to start STDIO server: %w", err)
	}

	return nil
}
