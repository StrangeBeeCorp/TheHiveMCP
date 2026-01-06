package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/server"
)

func GetHTTPAuthContextFunc(options *types.TheHiveMcpDefaultOptions) func(ctx context.Context, r *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		// Map header keys to context keys and environment variables
		type keyMap struct {
			header string
			ctxKey types.CtxKey
			deflt  string
		}
		keys := []keyMap{
			{"Authorization", types.HiveAPIKeyCtxKey, options.TheHiveAPIKey},
			{string(types.HeaderKeyTheHiveAPIKey), types.HiveAPIKeyCtxKey, options.TheHiveAPIKey},
			{string(types.HeaderKeyTheHiveOrganisation), types.HiveOrgCtxKey, options.TheHiveOrganisation},
			{string(types.HeaderKeyTheHiveURL), types.HiveURLCtxKey, options.TheHiveURL},
			{string(types.HeaderKeyOpenAIAPIKey), types.OpenAIAPIKeyCtxKey, options.OpenAIAPIKey},
			{string(types.HeaderKeyOpenAIBaseURL), types.OpenAIBaseURLCtxKey, options.OpenAIBaseURL},
			{string(types.HeaderKeyOpenAIModelName), types.OpenAIModelCtxKey, options.OpenAIModel},
		}

		// Extract string values into context
		for _, km := range keys {
			val := r.Header.Get(km.header)
			if val == "" {
				val = km.deflt
			}
			if val != "" {
				// Special handling for Authorization header
				if km.header == "Authorization" {
					val = ExtractBearerToken(val)
				}
				ctx = context.WithValue(ctx, km.ctxKey, val)
			}
		}

		// Handle max tokens header separately (integer value)
		maxTokensHeader := r.Header.Get(string(types.HeaderKeyOpenAIMaxTokens))
		maxTokens := options.OpenAIMaxTokens
		if maxTokensHeader != "" {
			if parsed, err := strconv.Atoi(maxTokensHeader); err == nil {
				maxTokens = parsed
			}
		}
		ctx = context.WithValue(ctx, types.OpenAIMaxTokensCtxKey, maxTokens)

		// Add Hive client to context using extracted credentials
		hiveAPIKey, _ := ctx.Value(types.HiveAPIKeyCtxKey).(string)
		hiveOrganisation, _ := ctx.Value(types.HiveOrgCtxKey).(string)
		hiveURL, _ := ctx.Value(types.HiveURLCtxKey).(string)

		if hiveURL != "" {
			creds := &TheHiveCredentials{
				URL:          hiveURL,
				APIKey:       hiveAPIKey,
				Username:     options.TheHiveUsername,
				Password:     options.TheHivePassword,
				Organisation: hiveOrganisation,
			}

			if newCtx, err := AddTheHiveClientToContextWithCreds(ctx, creds); err != nil {
				slog.Warn("Failed to add TheHive client to context", "error", err)
			} else {
				ctx = newCtx
			}
		}

		// Add OpenAI client to context using extracted configuration
		openAIAPIKey, _ := ctx.Value(types.OpenAIAPIKeyCtxKey).(string)
		openAIBaseURL, _ := ctx.Value(types.OpenAIBaseURLCtxKey).(string)
		openAIModel, _ := ctx.Value(types.OpenAIModelCtxKey).(string)
		openAIMaxTokens, _ := ctx.Value(types.OpenAIMaxTokensCtxKey).(int)

		// Only create OpenAI client if we have an API key
		if openAIAPIKey != "" {
			openAICreds := &OpenAICredentials{
				APIKey:    openAIAPIKey,
				BaseURL:   openAIBaseURL,
				Model:     openAIModel,
				MaxTokens: openAIMaxTokens,
			}

			// Set defaults if not provided
			if openAICreds.BaseURL == "" {
				openAICreds.BaseURL = "https://api.openai.com/v1"
			}
			if openAICreds.Model == "" {
				openAICreds.Model = "gpt-4"
			}

			if newCtx, err := AddOpenAIClientToContextWithCreds(ctx, openAICreds); err != nil {
				slog.Warn("Failed to add OpenAI client to context", "error", err)
			} else {
				ctx = newCtx
			}
		}

		// Add permissions to context
		if newCtx, err := AddPermissionsToContext(ctx, options); err != nil {
			slog.Warn("Failed to add permissions to context", "error", err)
		} else {
			ctx = newCtx
		}

		return ctx
	}
}

// StartHTTPServer starts the HTTP server with production-ready configuration
func StartHTTPServer(s *server.MCPServer, options *types.TheHiveMcpDefaultOptions) error {
	if s == nil {
		return fmt.Errorf("MCP server cannot be nil")
	}

	if options.BindAddr == "" {
		return fmt.Errorf("bind address cannot be empty")
	}

	var httpOptions []server.StreamableHTTPOption
	httpOptions = append(httpOptions, server.WithEndpointPath(options.MCPServerEndpointPath))
	httpOptions = append(httpOptions, server.WithStateLess(false))
	httpOptions = append(httpOptions, server.WithHTTPContextFunc(GetHTTPAuthContextFunc(options)))

	// Configure heartbeat interval if specified
	if options.MCPHeartbeatInterval != "" {
		if duration, err := time.ParseDuration(options.MCPHeartbeatInterval); err != nil {
			slog.Warn("Invalid heartbeat interval format, using default",
				"error", err,
				"interval", options.MCPHeartbeatInterval)
		} else {
			httpOptions = append(httpOptions, server.WithHeartbeatInterval(duration))
			slog.Info("Configured custom heartbeat interval", "interval", duration)
		}
	}

	httpServer := server.NewStreamableHTTPServer(s, httpOptions...)
	slog.Info("Starting HTTP server",
		"bind_addr", options.BindAddr,
		"endpoint", options.MCPServerEndpointPath,
		"stateless", false,
	)

	if err := httpServer.Start(options.BindAddr); err != nil {
		slog.Error("Failed to start HTTP server",
			"error", err,
			"bind_addr", options.BindAddr)
		return fmt.Errorf("failed to start HTTP server on %s: %w", options.BindAddr, err)
	}

	return nil
}
