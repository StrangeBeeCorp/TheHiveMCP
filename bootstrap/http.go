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

// resolveHiveCredsIntoContext extracts TheHive credential/URL headers (falling
// back to the configured env defaults when opted in) and stores them in the
// returned context.
func resolveHiveCredsIntoContext(ctx context.Context, r *http.Request, options *types.TheHiveMcpDefaultOptions) context.Context {
	// Env credentials are a fallback for requests carrying none, and only
	// when explicitly opted in.
	envAPIKey := ""

	if options.AllowEnvCredentialFallback {
		envAPIKey = options.TheHiveAPIKey
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

		if val == "" {
			continue
		}

		// Special handling for Authorization header
		if km.header == "Authorization" {
			val = ExtractBearerToken(val)
		}

		ctx = context.WithValue(ctx, km.ctxKey, val)
	}

	return ctx
}

// validateHiveAuthIntoContext enforces the URL allowlist and, when it passes,
// builds a TheHive client and validates the credentials upstream. It records
// the outcome (client + permissions on success, AuthErrorCtxKey on failure) in
// the returned context. Fail-closed: the middleware denies any request whose
// context lacks AuthValidatedCtxKey.
func validateHiveAuthIntoContext(ctx context.Context, allowlist *TheHiveURLAllowlist, allowlistErr error, cache *validationCache, options *types.TheHiveMcpDefaultOptions) context.Context {
	hiveAPIKey, _ := ctx.Value(types.HiveAPIKeyCtxKey).(string)
	hiveOrganisation, _ := ctx.Value(types.HiveOrgCtxKey).(string)
	hiveURL, _ := ctx.Value(types.HiveURLCtxKey).(string)

	switch {
	case allowlistErr != nil:
		return context.WithValue(ctx, types.AuthErrorCtxKey, errors.New("TheHive authentication failed: invalid TheHive URL allowlist configuration"))
	case hiveURL == "":
		return context.WithValue(ctx, types.AuthErrorCtxKey, errors.New("TheHive authentication failed: no TheHive URL provided"))
	case !allowlist.Allows(hiveURL):
		// Reject before any outbound request: never send creds to an
		// attacker-controlled destination (SSRF / credential disclosure).
		slog.Warn("Rejected TheHive URL not in allowlist", "url", hiveURL)

		return context.WithValue(ctx, types.AuthErrorCtxKey, errors.New("TheHive authentication failed: TheHive URL is not in the allowlist"))
	}

	envUsername := ""
	envPassword := ""

	if options.AllowEnvCredentialFallback {
		envUsername = options.TheHiveUsername
		envPassword = options.TheHivePassword
	}

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

		return context.WithValue(ctx, types.AuthErrorCtxKey, fmt.Errorf("TheHive authentication failed: %w", err))
	}

	return validateTheHiveAuthInContext(newCtx, creds, cache)
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
		ctx = resolveHiveCredsIntoContext(ctx, r, options)
		ctx = validateHiveAuthIntoContext(ctx, allowlist, allowlistErr, cache, options)

		if options.DefaultCortexID != "" {
			ctx = context.WithValue(ctx, types.DefaultCortexIDCtxKey, options.DefaultCortexID)
		}

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

	// Reject a bad allowlist at startup rather than denying every request at runtime.
	_, err := NewTheHiveURLAllowlist(options.TheHiveURLAllowlist, options.TheHiveURL)
	if err != nil {
		return fmt.Errorf("invalid TheHive URL allowlist configuration: %w", err)
	}

	var httpOptions []server.StreamableHTTPOption

	httpOptions = append(httpOptions, server.WithEndpointPath(options.MCPServerEndpointPath))
	httpOptions = append(httpOptions, server.WithStateLess(false))
	httpOptions = append(httpOptions, server.WithHTTPContextFunc(GetHTTPAuthContextFunc(options)))

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
