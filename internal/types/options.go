package types

import (
	"flag"
	"fmt"
	"os"

	"github.com/StrangeBeeCorp/TheHiveMCP/version"
)

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
	// OpenAIBaseURL is the base URL for OpenAI API (optional)
	OpenAIBaseURL string
	// OpenAIAPIKey is the API key for OpenAI
	OpenAIAPIKey string
	// OpenAIModel is the model to use for OpenAI
	OpenAIModel string
	// OpenAIMaxTokens is the maximum tokens for OpenAI responses (default: 320000)
	OpenAIMaxTokens int
}

func defaultToEnv(envKey EnvKey, defaultValue string) string {
	if value, exists := os.LookupEnv(string(envKey)); exists {
		return value
	}
	return defaultValue
}

func defaultToEnvInt(envKey EnvKey, defaultValue int) int {
	if value, exists := os.LookupEnv(string(envKey)); exists {
		var intValue int
		_, err := fmt.Sscanf(value, "%d", &intValue)
		if err == nil {
			return intValue
		}
	}
	return defaultValue
}

func NewTheHiveMcpDefaultOptions() (*TheHiveMcpDefaultOptions, error) {
	var showVersion bool
	var transport string
	var bindAddr string
	var theHiveURL string
	var theHiveAPIKey string
	var theHiveUsername string
	var theHivePassword string
	var theHiveOrganisation string
	var permissionsConfigPath string
	var mcpEndpointPath string
	var mcpHeartbeatInterval string
	var logLevel string
	var openAIBaseURL string
	var openAIAPIKey string
	var openAIModel string
	var openAIMaxTokens int
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.StringVar(&transport, string(FlagVarTransportType), "http", "Transport type (stdio, or http)")
	flag.StringVar(&bindAddr, string(FlagVarBindAddr), "", "Address to listen on for HTTP server (overrides env vars)")
	flag.StringVar(&theHiveURL, string(FlagVarTheHiveURL), defaultToEnv(EnvKeyTheHiveURL, ""), "TheHive URL (overrides env var THEHIVE_URL)")
	flag.StringVar(&theHiveAPIKey, string(FlagVarTheHiveAPIKey), defaultToEnv(EnvKeyTheHiveAPIKey, ""), "TheHive API key (overrides env var THEHIVE_API_KEY)")
	flag.StringVar(&theHiveUsername, string(FlagVarTheHiveUsername), defaultToEnv(EnvKeyTheHiveUsername, ""), "TheHive username for basic auth (overrides env var THEHIVE_USERNAME)")
	flag.StringVar(&theHivePassword, string(FlagVarTheHivePassword), defaultToEnv(EnvKeyTheHivePassword, ""), "TheHive password for basic auth (overrides env var THEHIVE_PASSWORD)")
	flag.StringVar(&theHiveOrganisation, string(FlagVarTheHiveOrganisation), defaultToEnv(EnvKeyTheHiveOrganisation, ""), "TheHive organisation (overrides env var THEHIVE_ORGANISATION)")
	flag.StringVar(&permissionsConfigPath, string(FlagVarPermissionsConfig), defaultToEnv(EnvKeyPermissionsConfig, ""), "Path to permissions config file (overrides env var PERMISSIONS_CONFIG, defaults to read-only)")
	flag.StringVar(&mcpEndpointPath, string(FlagVarMCPServerEndpointPath), defaultToEnv(EnvKeyMCPServerEndpoint, "/mcp"), "MCP server endpoint path (overrides env var HIVEMIND_MCP_ENDPOINT_PATH)")
	flag.StringVar(&mcpHeartbeatInterval, string(FlagVarMCPHeartbeatInterval), defaultToEnv(EnvKeyMCPHeartbeatInterval, "30s"), "MCP server heartbeat interval (overrides env var HIVEMIND_MCP_HEARTBEAT_INTERVAL)")
	flag.StringVar(&logLevel, string(FlagVarLogLevel), defaultToEnv(EnvKeyLogLevel, "info"), "Logging level (overrides env var LOG_LEVEL)")
	flag.StringVar(&openAIBaseURL, string(FlagVarOpenAIBaseURL), defaultToEnv(EnvKeyOpenAIBaseURL, "https://api.openai.com/v1"), "OpenAI base URL (overrides env var OPENAI_BASE_URL)")
	flag.StringVar(&openAIAPIKey, string(FlagVarOpenAIAPIKey), defaultToEnv(EnvKeyOpenAIAPIKey, ""), "OpenAI API key (overrides env var OPENAI_API_KEY)")
	flag.StringVar(&openAIModel, string(FlagVarOpenAIModel), defaultToEnv(EnvKeyOpenAIModel, "gpt-4"), "OpenAI model (overrides env var OPENAI_MODEL)")
	flag.IntVar(&openAIMaxTokens, string(FlagVarOpenAIMaxTokens), defaultToEnvInt(EnvKeyOpenAIMaxTokens, 32000), "OpenAI max tokens (overrides env var OPENAI_MAX_TOKENS)")
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
			return nil, fmt.Errorf("MCP server address and port must be set to use http mode, either via env vars MCP_URL and MCP_PORT, or by omitting the -addr flag")
		}
		bindAddr = fmt.Sprintf("%s:%s", host, port)
	}

	return &TheHiveMcpDefaultOptions{
		TheHiveURL:            theHiveURL,
		TheHiveAPIKey:         theHiveAPIKey,
		TheHiveUsername:       theHiveUsername,
		TheHivePassword:       theHivePassword,
		TheHiveOrganisation:   theHiveOrganisation,
		PermissionsConfigPath: permissionsConfigPath,
		MCPServerEndpointPath: mcpEndpointPath,
		MCPHeartbeatInterval:  mcpHeartbeatInterval,
		TransportType:         transport,
		BindAddr:              bindAddr,
		LogLevel:              logLevel,
		OpenAIBaseURL:         openAIBaseURL,
		OpenAIAPIKey:          openAIAPIKey,
		OpenAIModel:           openAIModel,
		OpenAIMaxTokens:       openAIMaxTokens,
	}, nil
}
