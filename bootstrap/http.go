package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/server"
)

func parseAuthValidationCacheTTL(ttl string) time.Duration {
	if ttl == "" {
		return DefaultAuthValidationCacheTTL
	}
	duration, err := time.ParseDuration(ttl)
	if err != nil || duration <= 0 {
		slog.Warn("Invalid auth validation cache TTL, using default",
			"ttl", ttl,
			"default", DefaultAuthValidationCacheTTL)
		return DefaultAuthValidationCacheTTL
	}
	return duration
}

func GetHTTPAuthContextFunc(options *types.TheHiveMcpDefaultOptions) func(ctx context.Context, r *http.Request) context.Context {
	allowlist, allowlistErr := NewTheHiveURLAllowlist(options.TheHiveURLAllowlist, options.TheHiveURL)
	if allowlistErr != nil {
		// Fail closed: with a nil allowlist every TheHive URL is rejected
		slog.Error("Invalid TheHive URL allowlist configuration, all HTTP requests will be rejected", "error", allowlistErr)
	}
	cache := newValidationCache(parseAuthValidationCacheTTL(options.AuthValidationCacheTTL))

	return func(ctx context.Context, r *http.Request) context.Context {
		// Env credentials are a fallback for requests carrying none, and only
		// when explicitly opted in.
		envAPIKey := ""
		envUsername := ""
		envPassword := ""
		if options.AllowEnvCredentialFallback {
			envAPIKey = options.TheHiveAPIKey
			envUsername = options.TheHiveUsername
			envPassword = options.TheHivePassword
		}

		type keyMap struct {
			header string
			ctxKey types.CtxKey
			deflt  string
		}
		keys := []keyMap{
			{"Authorization", types.HiveAPIKeyCtxKey, envAPIKey},
			{string(types.HeaderKeyTheHiveAPIKey), types.HiveAPIKeyCtxKey, envAPIKey},
			{string(types.HeaderKeyTheHiveOrganisation), types.HiveOrgCtxKey, options.TheHiveOrganisation},
			{string(types.HeaderKeyTheHiveURL), types.HiveURLCtxKey, options.TheHiveURL},
		}

		for _, km := range keys {
			val := r.Header.Get(km.header)
			if val == "" {
				val = km.deflt
			}
			if val != "" {
				if km.header == "Authorization" {
					val = ExtractBearerToken(val)
				}
				ctx = context.WithValue(ctx, km.ctxKey, val)
			}
		}

		// Fail-closed: AuthValidatedCtxKey is set only after the URL passes the
		// allowlist and creds validate; the middleware denies requests lacking it.
		hiveAPIKey, _ := ctx.Value(types.HiveAPIKeyCtxKey).(string)
		hiveOrganisation, _ := ctx.Value(types.HiveOrgCtxKey).(string)
		hiveURL, _ := ctx.Value(types.HiveURLCtxKey).(string)

		switch {
		case allowlistErr != nil:
			ctx = context.WithValue(ctx, types.AuthErrorCtxKey, fmt.Errorf("TheHive authentication failed: invalid TheHive URL allowlist configuration"))
		case hiveURL == "":
			ctx = context.WithValue(ctx, types.AuthErrorCtxKey, fmt.Errorf("TheHive authentication failed: no TheHive URL provided"))
		case !allowlist.Allows(hiveURL):
			// Reject before any outbound request: never send creds to an
			// attacker-controlled destination (SSRF / credential disclosure).
			slog.Warn("Rejected TheHive URL not in allowlist", "url", hiveURL)
			ctx = context.WithValue(ctx, types.AuthErrorCtxKey, fmt.Errorf("TheHive authentication failed: TheHive URL is not in the allowlist"))
		default:
			creds := &TheHiveCredentials{
				URL:          hiveURL,
				APIKey:       hiveAPIKey,
				Username:     envUsername,
				Password:     envPassword,
				Organisation: hiveOrganisation,
			}

			if newCtx, err := AddTheHiveClientToContextWithCreds(ctx, creds); err != nil {
				slog.Error("Failed to add TheHive client to context", "error", err)
				ctx = context.WithValue(ctx, types.AuthErrorCtxKey, fmt.Errorf("TheHive authentication failed: %w", err))
			} else {
				ctx = validateTheHiveAuthInContext(newCtx, creds, cache)
			}
		}

		if options.DefaultCortexID != "" {
			ctx = context.WithValue(ctx, types.DefaultCortexIDCtxKey, options.DefaultCortexID)
		}

		if newCtx, err := AddPermissionsToContext(ctx, options); err != nil {
			slog.Warn("Failed to add permissions to context", "error", err)
		} else {
			ctx = newCtx
		}

		return ctx
	}
}

func StartHTTPServer(s *server.MCPServer, options *types.TheHiveMcpDefaultOptions) error {
	if s == nil {
		return fmt.Errorf("MCP server cannot be nil")
	}

	if options.BindAddr == "" {
		return fmt.Errorf("bind address cannot be empty")
	}

	// Reject a bad allowlist at startup rather than denying every request at runtime.
	if _, err := NewTheHiveURLAllowlist(options.TheHiveURLAllowlist, options.TheHiveURL); err != nil {
		return fmt.Errorf("invalid TheHive URL allowlist configuration: %w", err)
	}

	var httpOptions []server.StreamableHTTPOption
	httpOptions = append(httpOptions, server.WithEndpointPath(options.MCPServerEndpointPath))
	httpOptions = append(httpOptions, server.WithStateLess(false))
	httpOptions = append(httpOptions, server.WithHTTPContextFunc(GetHTTPAuthContextFunc(options)))

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
