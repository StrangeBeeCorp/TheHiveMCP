package logging

import (
	"log/slog"
	"os"
)

func ParseLevel(s string) (slog.Level, error) {
	var level slog.Level
	var err = level.UnmarshalText([]byte(s))
	return level, err
}

func InitLogger(levelStr string, transportType string) *slog.Logger {
	level, err := ParseLevel(levelStr)
	if err != nil {
		slog.Error("Invalid log level", "level", levelStr, "error", err)
		os.Exit(1)
	}

	// Choose output stream based on transport type
	output := os.Stdout
	if transportType == "stdio" {
		// In STDIO mode, stdout is reserved for JSON-RPC communication
		// All logs must go to stderr to avoid interfering with MCP protocol
		output = os.Stderr
	}

	logger := slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
	return logger
}
