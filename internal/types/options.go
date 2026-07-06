package types

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/version"
)

// TheHiveMcpDefaultOptions holds the resolved server configuration built from
// CLI flags and environment variables.
type TheHiveMcpDefaultOptions struct {
	// TheHiveURL is the URL of TheHive instance
	TheHiveURL string
	// TheHiveAPIKey is the API key for TheHive
	TheHiveAPIKey string
	// TheHiveUsername is the username for TheHive (for basic auth)
	TheHiveUsername string
	// TheHivePassword is the password for TheHive (for basic auth)
	TheHivePassword string
	// TheHiveOrganisation is the organisation for TheHive (optional)
	TheHiveOrganisation string
	// TheHiveURLAllowlist is the list of TheHive base URLs HTTP clients may target via the
	// X-TheHive-Url header; when empty, only TheHiveURL is permitted
	TheHiveURLAllowlist []string
	// AllowEnvCredentialFallback allows HTTP requests without credentials to fall back to the
	// server's environment credentials (default: false, requests must supply their own credentials)
	AllowEnvCredentialFallback bool
	// AuthValidationCacheTTL is how long a successful TheHive credential validation is cached
	// for the HTTP transport (default: 60s)
	AuthValidationCacheTTL string
	// PermissionsConfigPath is the path to the permissions configuration file (optional, defaults to embedded read-only config)
	PermissionsConfigPath string
	// MCPServerEndpointPath is the endpoint path for the MCP server (default: /mcp)
	MCPServerEndpointPath string
	// MCPHeartbeatInterval is the heartbeat interval for the MCP server (default: 30s)
	MCPHeartbeatInterval string
	// TransportType is the transport type for the MCP server (default: http)
	TransportType string
	// BindAddr is the address to bind the HTTP server to (if using HTTP transport)
	BindAddr string
	// LogLevel is the logging level for the application
	LogLevel string
	// DefaultCortexID is the default Cortex instance ID used when none is specified (default: local)
	DefaultCortexID string
}

func defaultToEnv(envKey EnvKey, defaultValue string) string {
	if value, exists := os.LookupEnv(string(envKey)); exists {
		return value
	}

	return defaultValue
}

func defaultToEnvBool(envKey EnvKey, defaultValue bool) bool {
	if value, exists := os.LookupEnv(string(envKey)); exists {
		boolValue, err := strconv.ParseBool(value)
		if err == nil {
			return boolValue
		}
	}

	return defaultValue
}

// splitCommaSeparated splits a comma-separated string into trimmed, non-empty entries
func splitCommaSeparated(value string) []string {
	var entries []string

	for entry := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			entries = append(entries, trimmed)
		}
	}

	return entries
}

// NewTheHiveMcpDefaultOptions parses CLI flags (falling back to environment
// variables) and returns the resolved server options.
func NewTheHiveMcpDefaultOptions() (*TheHiveMcpDefaultOptions, error) {
	var (
		showVersion                bool
		transport                  string
		bindAddr                   string
		theHiveURL                 string
		theHiveAPIKey              string
		theHiveUsername            string
		theHivePassword            string
		theHiveOrganisation        string
		theHiveURLAllowlist        string
		allowEnvCredentialFallback bool
		authValidationCacheTTL     string
		permissionsConfigPath      string
		mcpEndpointPath            string
		mcpHeartbeatInterval       string
		logLevel                   string
		cortexID                   string
	)

	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.StringVar(&transport, string(FlagVarTransportType), "http", "Transport type (stdio, or http)")
	flag.StringVar(&bindAddr, string(FlagVarBindAddr), "", "Address to listen on for HTTP server (overrides env vars)")
	flag.StringVar(&theHiveURL, string(FlagVarTheHiveURL), defaultToEnv(EnvKeyTheHiveURL, ""), "TheHive URL (overrides env var THEHIVE_URL)")
	flag.StringVar(&theHiveAPIKey, string(FlagVarTheHiveAPIKey), defaultToEnv(EnvKeyTheHiveAPIKey, ""), "TheHive API key (overrides env var THEHIVE_API_KEY)")
	flag.StringVar(&theHiveUsername, string(FlagVarTheHiveUsername), defaultToEnv(EnvKeyTheHiveUsername, ""), "TheHive username for basic auth (overrides env var THEHIVE_USERNAME)")
	flag.StringVar(&theHivePassword, string(FlagVarTheHivePassword), defaultToEnv(EnvKeyTheHivePassword, ""), "TheHive password for basic auth (overrides env var THEHIVE_PASSWORD)")
	flag.StringVar(&theHiveOrganisation, string(FlagVarTheHiveOrganisation), defaultToEnv(EnvKeyTheHiveOrganisation, ""), "TheHive organisation (overrides env var THEHIVE_ORGANISATION)")
	flag.StringVar(&theHiveURLAllowlist, string(FlagVarTheHiveURLAllowlist), defaultToEnv(EnvKeyTheHiveURLAllowlist, ""), "Comma-separated list of TheHive base URLs HTTP clients may target via the X-TheHive-Url header; defaults to the TheHive URL only (overrides env var THEHIVE_URL_ALLOWLIST)")
	flag.BoolVar(&allowEnvCredentialFallback, string(FlagVarAllowEnvCredentialFallback), defaultToEnvBool(EnvKeyAllowEnvCredentialFallback, false), "Allow HTTP requests without credentials to fall back to the server's environment credentials (overrides env var ALLOW_ENV_CREDENTIAL_FALLBACK, default false)")
	flag.StringVar(&authValidationCacheTTL, string(FlagVarAuthValidationCacheTTL), defaultToEnv(EnvKeyAuthValidationCacheTTL, "60s"), "TTL for cached TheHive credential validation results on the HTTP transport (overrides env var AUTH_VALIDATION_CACHE_TTL)")
	flag.StringVar(&permissionsConfigPath, string(FlagVarPermissionsConfig), defaultToEnv(EnvKeyPermissionsConfig, ""), "Path to permissions config file (overrides env var PERMISSIONS_CONFIG, defaults to read-only)")
	flag.StringVar(&mcpEndpointPath, string(FlagVarMCPServerEndpointPath), defaultToEnv(EnvKeyMCPServerEndpoint, "/mcp"), "MCP server endpoint path (overrides env var HIVEMIND_MCP_ENDPOINT_PATH)")
	flag.StringVar(&mcpHeartbeatInterval, string(FlagVarMCPHeartbeatInterval), defaultToEnv(EnvKeyMCPHeartbeatInterval, "30s"), "MCP server heartbeat interval (overrides env var HIVEMIND_MCP_HEARTBEAT_INTERVAL)")
	flag.StringVar(&logLevel, string(FlagVarLogLevel), defaultToEnv(EnvKeyLogLevel, "info"), "Logging level (overrides env var LOG_LEVEL)")
	flag.StringVar(&cortexID, string(FlagVarCortexID), defaultToEnv(EnvKeyCortexID, DefaultCortexID), "Default Cortex instance ID (overrides env var CORTEX_ID, defaults to 'local')")
	flag.Parse()

	// Handle version flag
	if showVersion {
		fmt.Printf("TheHiveMCP %s\n", version.Info())
		os.Exit(0)
	}

	if bindAddr == "" && transport == "http" {
		host := os.Getenv(string(EnvKeyBindHost))

		port := os.Getenv(string(EnvKeyMCPPort))
		if host == "" || port == "" {
			return nil, errors.New("MCP server address and port must be set to use http mode, either via env vars MCP_URL and MCP_PORT, or by omitting the -addr flag")
		}

		bindAddr = fmt.Sprintf("%s:%s", host, port)
	}

	return &TheHiveMcpDefaultOptions{
		TheHiveURL:                 theHiveURL,
		TheHiveAPIKey:              theHiveAPIKey,
		TheHiveUsername:            theHiveUsername,
		TheHivePassword:            theHivePassword,
		TheHiveOrganisation:        theHiveOrganisation,
		TheHiveURLAllowlist:        splitCommaSeparated(theHiveURLAllowlist),
		AllowEnvCredentialFallback: allowEnvCredentialFallback,
		AuthValidationCacheTTL:     authValidationCacheTTL,
		PermissionsConfigPath:      permissionsConfigPath,
		MCPServerEndpointPath:      mcpEndpointPath,
		MCPHeartbeatInterval:       mcpHeartbeatInterval,
		TransportType:              transport,
		BindAddr:                   bindAddr,
		LogLevel:                   logLevel,
		DefaultCortexID:            cortexID,
	}, nil
}
