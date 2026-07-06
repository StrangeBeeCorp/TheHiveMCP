package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// parseAuthValidationCacheTTL parses the configured validation cache TTL,
// falling back to the default on empty or invalid values.
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

// GetHTTPAuthContextFunc returns an HTTP context function that extracts TheHive
// credentials and target URL from request headers (or configured fallbacks),
// enforces the URL allowlist, validates the credentials upstream (with caching),
// and stores the resulting client, permissions, and auth outcome in the context.
func GetHTTPAuthContextFunc(options *types.TheHiveMcpDefaultOptions) func(ctx context.Context, r *http.Request) context.Context {
	allowlist, allowlistErr := NewTheHiveURLAllowlist(options.TheHiveURLAllowlist, options.TheHiveURL)
	if allowlistErr != nil {
		// Fail closed: with a nil allowlist every TheHive URL is rejected
		slog.Error("Invalid TheHive URL allowlist configuration, all HTTP requests will be rejected", "error", allowlistErr)
	}

	cache := newValidationCache(parseAuthValidationCacheTTL(options.AuthValidationCacheTTL))

	return func(ctx context.Context, r *http.Request) context.Context {
		// Credentials from the server environment are only used as a fallback
		// for requests that carry none of their own when explicitly opted in
		envAPIKey := ""
		envUsername := ""
		envPassword := ""

		if options.AllowEnvCredentialFallback {
			envAPIKey = options.TheHiveAPIKey
			envUsername = options.TheHiveUsername
			envPassword = options.TheHivePassword
		}

		// Map header keys to context keys and environment variables
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

		// Resolve every header value into a flat key->value map first, so the
		// context is augmented outside the loop and we avoid nesting
		// context.WithValue calls across iterations.
		resolved := make(map[types.CtxKey]string, len(keys))

		for _, km := range keys {
			val := r.Header.Get(km.header)
			if val == "" {
				val = km.deflt
			}

			if val == "" {
				continue
			}

			// Special handling for Authorization header
			if km.header == "Authorization" {
				val = ExtractBearerToken(val)
			}

			resolved[km.ctxKey] = val
		}

		if v, ok := resolved[types.HiveAPIKeyCtxKey]; ok {
			ctx = context.WithValue(ctx, types.HiveAPIKeyCtxKey, v)
		}

		if v, ok := resolved[types.HiveOrgCtxKey]; ok {
			ctx = context.WithValue(ctx, types.HiveOrgCtxKey, v)
		}

		if v, ok := resolved[types.HiveURLCtxKey]; ok {
			ctx = context.WithValue(ctx, types.HiveURLCtxKey, v)
		}

		// Add Hive client to context using extracted credentials. Authentication
		// is fail-closed: types.AuthValidatedCtxKey is only set after the TheHive
		// URL passed the allowlist and the credentials were validated; the
		// middleware denies any request without that marker.
		hiveAPIKey, _ := ctx.Value(types.HiveAPIKeyCtxKey).(string)
		hiveOrganisation, _ := ctx.Value(types.HiveOrgCtxKey).(string)
		hiveURL, _ := ctx.Value(types.HiveURLCtxKey).(string)

		switch {
		case allowlistErr != nil:
			ctx = context.WithValue(ctx, types.AuthErrorCtxKey, errors.New("TheHive authentication failed: invalid TheHive URL allowlist configuration"))
		case hiveURL == "":
			ctx = context.WithValue(ctx, types.AuthErrorCtxKey, errors.New("TheHive authentication failed: no TheHive URL provided"))
		case !allowlist.Allows(hiveURL):
			// Reject before any outbound request so credentials are never sent
			// to an attacker-controlled destination (SSRF / credential disclosure)
			slog.Warn("Rejected TheHive URL not in allowlist", "url", hiveURL)

			ctx = context.WithValue(ctx, types.AuthErrorCtxKey, errors.New("TheHive authentication failed: TheHive URL is not in the allowlist"))
		default:
			creds := &TheHiveCredentials{
				URL:          hiveURL,
				APIKey:       hiveAPIKey,
				Username:     envUsername,
				Password:     envPassword,
				Organisation: hiveOrganisation,
			}

			newCtx, err := AddTheHiveClientToContextWithCreds(ctx, creds)
			if err != nil {
				slog.Error("Failed to add TheHive client to context", "error", err)
				ctx = context.WithValue(ctx, types.AuthErrorCtxKey, fmt.Errorf("TheHive authentication failed: %w", err))
			} else {
				// Validate TheHive client credentials, skipping the upstream call
				// when the same credentials were validated recently
				ctx = validateTheHiveAuthInContext(newCtx, creds, cache)
			}
		}

		// Add default Cortex ID to context
		if options.DefaultCortexID != "" {
			ctx = context.WithValue(ctx, types.DefaultCortexIDCtxKey, options.DefaultCortexID)
		}

		// Add permissions to context
		permsCtx, err := AddPermissionsToContext(ctx, options)
		if err != nil {
			slog.Warn("Failed to add permissions to context", "error", err)
		} else {
			ctx = permsCtx
		}

		return ctx
	}
}

// StartHTTPServer starts the HTTP server with production-ready configuration
func StartHTTPServer(s *server.MCPServer, options *types.TheHiveMcpDefaultOptions) error {
	if s == nil {
		return errors.New("MCP server cannot be nil")
	}

	if options.BindAddr == "" {
		return errors.New("bind address cannot be empty")
	}

	// Reject invalid allowlist configuration at startup rather than denying
	// every request at runtime
	_, err := NewTheHiveURLAllowlist(options.TheHiveURLAllowlist, options.TheHiveURL)
	if err != nil {
		return fmt.Errorf("invalid TheHive URL allowlist configuration: %w", err)
	}

	var httpOptions []server.StreamableHTTPOption

	httpOptions = append(httpOptions, server.WithEndpointPath(options.MCPServerEndpointPath))
	httpOptions = append(httpOptions, server.WithStateLess(false))
	httpOptions = append(httpOptions, server.WithHTTPContextFunc(GetHTTPAuthContextFunc(options)))

	// Configure heartbeat interval if specified
	if options.MCPHeartbeatInterval != "" {
		duration, parseErr := time.ParseDuration(options.MCPHeartbeatInterval)
		if parseErr != nil {
			slog.Warn("Invalid heartbeat interval format, using default",
				"error", parseErr,
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

	err = httpServer.Start(options.BindAddr)
	if err != nil {
		slog.Error("Failed to start HTTP server",
			"error", err,
			"bind_addr", options.BindAddr)

		return fmt.Errorf("failed to start HTTP server on %s: %w", options.BindAddr, err)
	}

	return nil
}
