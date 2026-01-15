package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	openai "github.com/sashabaranov/go-openai"
)

// Common errors
var (
	ErrMissingHiveURL        = errors.New("THEHIVE_URL environment variable is required")
	ErrInvalidHiveURL        = errors.New("invalid TheHive URL format")
	ErrMissingAuthentication = errors.New("either API key or username/password must be provided")
	ErrInvalidAPIKey         = errors.New("API key cannot be empty or 'dummy'")
	ErrMissingCredentials    = errors.New("both username and password are required for basic auth")
)

// TheHiveCredentials holds authentication information for TheHive
type TheHiveCredentials struct {
	URL          string
	APIKey       string
	Username     string
	Password     string
	Organisation string
}

// OpenAICredentials holds authentication and configuration information for OpenAI
type OpenAICredentials struct {
	APIKey    string
	BaseURL   string
	Model     string
	MaxTokens int
}

// Validate validates the credentials
func (c *TheHiveCredentials) Validate() error {
	if c.URL == "" {
		return ErrMissingHiveURL
	}

	// Validate URL format
	if _, err := url.ParseRequestURI(c.URL); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidHiveURL, err)
	}

	// Check authentication method
	hasAPIKey := c.APIKey != "" && strings.ToLower(c.APIKey) != "dummy"
	hasBasicAuth := c.Username != "" && c.Password != ""

	if !hasAPIKey && !hasBasicAuth {
		return ErrMissingAuthentication
	}

	if c.APIKey != "" && strings.ToLower(c.APIKey) == "dummy" {
		return ErrInvalidAPIKey
	}

	if (c.Username != "" && c.Password == "") || (c.Username == "" && c.Password != "") {
		return ErrMissingCredentials
	}

	return nil
}

// Validate validates the OpenAI credentials
func (c *OpenAICredentials) Validate() error {
	// OpenAI credentials are optional, so we don't require an API key
	// However, if provided, it should not be empty or dummy
	if c.APIKey != "" && strings.ToLower(c.APIKey) == "dummy" {
		return errors.New("OpenAI API key cannot be 'dummy'")
	}

	// Validate base URL format if provided
	if c.BaseURL != "" {
		if _, err := url.ParseRequestURI(c.BaseURL); err != nil {
			return fmt.Errorf("invalid OpenAI base URL format: %v", err)
		}
	}

	return nil
}

// LoadTheHiveCredentialsFromEnv loads TheHive credentials from environment variables
func LoadTheHiveCredentialsFromEnv() (*TheHiveCredentials, error) {
	creds := &TheHiveCredentials{
		URL:          os.Getenv(string(types.EnvKeyTheHiveURL)),
		APIKey:       os.Getenv(string(types.EnvKeyTheHiveAPIKey)),
		Username:     os.Getenv(string(types.EnvKeyTheHiveUsername)),
		Password:     os.Getenv(string(types.EnvKeyTheHivePassword)),
		Organisation: os.Getenv(string(types.EnvKeyTheHiveOrganisation)),
	}

	if err := creds.Validate(); err != nil {
		return nil, fmt.Errorf("invalid TheHive credentials: %w", err)
	}

	return creds, nil
}

// LoadOpenAICredentialsFromEnv loads OpenAI credentials from environment variables
func LoadOpenAICredentialsFromEnv() (*OpenAICredentials, error) {
	maxTokensStr := os.Getenv(string(types.EnvKeyOpenAIMaxTokens))
	maxTokens := 32000 // default value
	if maxTokensStr != "" {
		if parsed, err := strconv.Atoi(maxTokensStr); err == nil {
			maxTokens = parsed
		}
	}

	creds := &OpenAICredentials{
		APIKey:    os.Getenv(string(types.EnvKeyOpenAIAPIKey)),
		BaseURL:   os.Getenv(string(types.EnvKeyOpenAIBaseURL)),
		Model:     os.Getenv(string(types.EnvKeyOpenAIModel)),
		MaxTokens: maxTokens,
	}

	// Set default values if not provided
	if creds.BaseURL == "" {
		creds.BaseURL = "https://api.openai.com/v1"
	}
	if creds.Model == "" {
		creds.Model = "gpt-4"
	}

	if err := creds.Validate(); err != nil {
		return nil, fmt.Errorf("invalid OpenAI credentials: %w", err)
	}

	return creds, nil
}

// CreateTheHiveConfig creates a TheHive configuration from credentials
func CreateTheHiveConfig(creds *TheHiveCredentials) (*thehive.Configuration, error) {
	if err := creds.Validate(); err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Transport: &utils.ElicitationTransport{
			Transport: &logging.LoggingTransport{
				Transport: http.DefaultTransport,
			},
		},
	}

	clientCfg := thehive.NewConfiguration()
	clientCfg.HTTPClient = httpClient
	baseURL := strings.TrimSuffix(creds.URL, "/")

	clientCfg.Servers = thehive.ServerConfigurations{
		{
			URL:         baseURL,
			Description: "TheHive Server",
		},
	}

	clientCfg.AddDefaultHeader("X-Organisation", creds.Organisation)

	return clientCfg, nil
}

// CreateTheHiveClient creates a TheHive client from credentials
func CreateTheHiveClient(creds *TheHiveCredentials) (*thehive.APIClient, error) {
	clientCfg, err := CreateTheHiveConfig(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to create TheHive config: %w", err)
	}
	slog.Info("Created TheHive client",
		"url", creds.URL,
		"organisation", creds.Organisation,
		"using_api_key", creds.APIKey != "")
	client := thehive.NewAPIClient(clientCfg)
	return client, nil
}

func addTheHiveAuthToContext(ctx context.Context, client *thehive.APIClient, creds *TheHiveCredentials) context.Context {
	if creds.APIKey != "" && strings.ToLower(creds.APIKey) != "dummy" {
		ctx = context.WithValue(ctx, thehive.ContextAccessToken, creds.APIKey)
	} else if creds.Username != "" && creds.Password != "" {
		basicAuth := thehive.BasicAuth{
			UserName: creds.Username,
			Password: creds.Password,
		}
		ctx = context.WithValue(ctx, thehive.ContextBasicAuth, basicAuth)
	}
	return ctx
}

// AddTheHiveClientToContext adds a TheHive client to the context using environment variables
func AddTheHiveClientToContext(ctx context.Context) (context.Context, error) {
	creds, err := LoadTheHiveCredentialsFromEnv()
	if err != nil {
		return ctx, fmt.Errorf("failed to load TheHive credentials: %w", err)
	}

	client, err := CreateTheHiveClient(creds)
	if err != nil {
		return ctx, fmt.Errorf("failed to create TheHive client: %w", err)
	}

	ctx = addTheHiveAuthToContext(ctx, client, creds)
	return context.WithValue(ctx, types.HiveClientCtxKey, client), nil
}

// AddTheHiveClientToContextWithCreds adds a TheHive client to the context using provided credentials
func AddTheHiveClientToContextWithCreds(ctx context.Context, creds *TheHiveCredentials) (context.Context, error) {
	client, err := CreateTheHiveClient(creds)
	if err != nil {
		return ctx, fmt.Errorf("failed to create TheHive client: %w", err)
	}

	ctx = addTheHiveAuthToContext(ctx, client, creds)
	return context.WithValue(ctx, types.HiveClientCtxKey, client), nil
}

// ExtractBearerToken extracts a bearer token from an Authorization header
func ExtractBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}

	const bearerPrefix = "bearer "
	if strings.HasPrefix(strings.ToLower(authHeader), bearerPrefix) {
		return strings.TrimSpace(authHeader[len(bearerPrefix):])
	}

	return authHeader
}

// CreateOpenAIClient creates an OpenAI client from credentials
func CreateOpenAIClient(creds *OpenAICredentials) (*openai.Client, error) {
	if err := creds.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate OpenAI credentials: %w", err)
	}

	config := openai.DefaultConfig(creds.APIKey)
	if creds.BaseURL != "" {
		config.BaseURL = creds.BaseURL
	}

	client := openai.NewClientWithConfig(config)
	slog.Info("Created OpenAI client",
		"base_url", creds.BaseURL,
		"model", creds.Model,
		"max_tokens", creds.MaxTokens,
		"has_api_key", creds.APIKey != "")
	return client, nil
}

// CreateOpenAIWrapper creates an OpenAI wrapper from credentials
func CreateOpenAIWrapper(creds *OpenAICredentials) (*utils.OpenAIWrapper, error) {
	client, err := CreateOpenAIClient(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI client: %w", err)
	}

	wrapper := &utils.OpenAIWrapper{
		Client:     client,
		ModelName:  creds.Model,
		MaxTokens:  creds.MaxTokens,
		MaxRetries: types.DefaultMaxCompletionRetries,
	}

	return wrapper, nil
}

// AddOpenAIClientToContext adds an OpenAI client to the context using environment variables
func AddOpenAIClientToContext(ctx context.Context) (context.Context, error) {
	creds, err := LoadOpenAICredentialsFromEnv()
	if err != nil {
		return ctx, fmt.Errorf("failed to load OpenAI credentials: %w", err)
	}

	// If no API key is provided, we don't add OpenAI to context (it's optional)
	if creds.APIKey == "" {
		slog.Debug("No OpenAI API key provided, skipping OpenAI client creation")
		return ctx, nil
	}

	wrapper, err := CreateOpenAIWrapper(creds)
	if err != nil {
		return ctx, fmt.Errorf("failed to create OpenAI wrapper: %w", err)
	}

	// Add OpenAI configuration to context
	ctx = context.WithValue(ctx, types.OpenAIAPIKeyCtxKey, creds.APIKey)
	ctx = context.WithValue(ctx, types.OpenAIBaseURLCtxKey, creds.BaseURL)
	ctx = context.WithValue(ctx, types.OpenAIModelCtxKey, creds.Model)
	ctx = context.WithValue(ctx, types.OpenAIMaxTokensCtxKey, creds.MaxTokens)

	return context.WithValue(ctx, types.OpenAIClientCtxKey, wrapper), nil
}

// AddOpenAIClientToContextWithCreds adds an OpenAI client to the context using provided credentials
func AddOpenAIClientToContextWithCreds(ctx context.Context, creds *OpenAICredentials) (context.Context, error) {
	// If no API key is provided, we don't add OpenAI to context (it's optional)
	if creds.APIKey == "" {
		slog.Debug("No OpenAI API key provided, skipping OpenAI client creation")
		return ctx, nil
	}

	wrapper, err := CreateOpenAIWrapper(creds)
	if err != nil {
		return ctx, fmt.Errorf("failed to create OpenAI wrapper: %w", err)
	}

	// Add OpenAI configuration to context
	ctx = context.WithValue(ctx, types.OpenAIAPIKeyCtxKey, creds.APIKey)
	ctx = context.WithValue(ctx, types.OpenAIBaseURLCtxKey, creds.BaseURL)
	ctx = context.WithValue(ctx, types.OpenAIModelCtxKey, creds.Model)
	ctx = context.WithValue(ctx, types.OpenAIMaxTokensCtxKey, creds.MaxTokens)

	return context.WithValue(ctx, types.OpenAIClientCtxKey, wrapper), nil
}

// SafeGetEnv gets an environment variable with optional default value
func SafeGetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// LoadPermissions loads permissions configuration from file or uses default
// Special values: "admin" for full permissions, "read_only" for default read-only permissions
// Empty string defaults to read-only, otherwise loads from the specified file path
func LoadPermissions(configPath string) (*permissions.Config, error) {
	// Special handling for special config values
	if configPath == string(types.PermissionConfigAdmin) {
		slog.Info("Using admin permissions for testing")
		config := permissions.LoadAdminForTesting()
		return config, nil
	}

	if configPath == string(types.PermissionConfigReadOnly) {
		slog.Info("Using default read-only permissions")
		config, err := permissions.LoadDefault()
		if err != nil {
			return nil, fmt.Errorf("failed to load default permissions: %w", err)
		}
		slog.Info("Default permissions loaded", "version", config.Version)
		return config, nil
	}

	if configPath != "" {
		slog.Info("Loading permissions from file", "path", configPath)
		config, err := permissions.LoadFromFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load permissions from file: %w", err)
		}
		slog.Info("Permissions loaded from file", "path", configPath, "version", config.Version)
		return config, nil
	}

	// Use embedded default permissions (empty string case)
	slog.Info("Using default read-only permissions")
	config, err := permissions.LoadDefault()
	if err != nil {
		return nil, fmt.Errorf("failed to load default permissions: %w", err)
	}
	slog.Info("Default permissions loaded", "version", config.Version)
	return config, nil
}

// AddPermissionsToContext loads permissions and adds them to the context
func AddPermissionsToContext(ctx context.Context, options *types.TheHiveMcpDefaultOptions) (context.Context, error) {
	config, err := LoadPermissions(options.PermissionsConfigPath)
	if err != nil {
		return ctx, fmt.Errorf("failed to load permissions: %w", err)
	}

	return utils.AddPermissionsToContext(ctx, config), nil
}
