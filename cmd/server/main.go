package main

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"

	"github.com/StrangeBeeCorp/TheHiveMCP/bootstrap"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/TheHiveMCP/version"
)

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Warn("No .env file found, proceeding with environment variables")
	}
	options, err := types.NewTheHiveMcpDefaultOptions()
	if err != nil {
		slog.Error("Failed to get default options", "error", err)
		os.Exit(1)
	}
	logging.InitLogger(options.LogLevel, options.TransportType)

	slog.Info("Starting TheHiveMCP server", "version", version.GitVersion())

	// Initialize OpenAI if configured (warnings only, no crash)
	utils.InitOpenai(options)

	mcpServer := bootstrap.GetMCPServerAndRegisterTools()

	switch options.TransportType {
	case "stdio":
		if err := bootstrap.StartStdioServer(mcpServer, options); err != nil {
			slog.Error("Failed to start STDIO server", "error", err)
			os.Exit(1)
		}
	case "http":
		if err := bootstrap.StartHTTPServer(mcpServer, options); err != nil {
			slog.Error("Failed to start HTTP server", "error", err)
			os.Exit(1)
		}
	}
}
