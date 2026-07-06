// Package main is the entry point for the TheHiveMCP server binary. It loads
// configuration, initialises logging, and serves the MCP server over the
// configured transport (stdio or http).
package main

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"

	"github.com/StrangeBeeCorp/TheHiveMCP/bootstrap"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/version"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		slog.Warn("No .env file found, proceeding with environment variables")
	}

	options, err := types.NewTheHiveMcpDefaultOptions()
	if err != nil {
		slog.Error("Failed to get default options", "error", err)
		os.Exit(1)
	}
	// Initialise the logger befor serving any requests.
	logging.InitLogger(options.LogLevel, options.TransportType)

	slog.Info("Starting TheHiveMCP server", "version", version.GitVersion())

	mcpServer := bootstrap.GetMCPServerAndRegisterTools()

	switch options.TransportType {
	case "stdio":
		err := bootstrap.StartStdioServer(mcpServer, options)
		if err != nil {
			slog.Error("Failed to start STDIO server", "error", err)
			os.Exit(1)
		}
	case "http":
		err := bootstrap.StartHTTPServer(mcpServer, options)
		if err != nil {
			slog.Error("Failed to start HTTP server", "error", err)
			os.Exit(1)
		}
	}
}
