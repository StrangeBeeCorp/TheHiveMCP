package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/logging"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// dummyAPIKey is the placeholder API key value that is treated as unset.
const dummyAPIKey = "dummy"

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

// Validate requires a valid URL plus either an API key or a username/password pair.
func (c *TheHiveCredentials) Validate() error {
	if c.URL == "" {
		return ErrMissingHiveURL
	}

	_, err := url.ParseRequestURI(c.URL)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidHiveURL, err)
	}

	hasAPIKey := c.APIKey != ""
	hasBasicAuth := c.Username != "" && c.Password != ""

	if !hasAPIKey && !hasBasicAuth {
		return ErrMissingAuthentication
	}

	if strings.ToLower(c.APIKey) == dummyAPIKey {
		return ErrInvalidAPIKey
	}

	if (c.Username != "" && c.Password == "") || (c.Username == "" && c.Password != "") {
		return ErrMissingCredentials
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

	err := creds.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid TheHive credentials: %w", err)
	}

	return creds, nil
}

// CreateTheHiveConfig creates a TheHive configuration from credentials
func CreateTheHiveConfig(creds *TheHiveCredentials) (*thehive.Configuration, error) {
	err := creds.Validate()
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Transport: &utils.ElicitationTransport{
			Transport: &logging.Transport{
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

	if creds.Organisation != "" {
		clientCfg.AddDefaultHeader("X-Organisation", creds.Organisation)
	}

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

func addTheHiveAuthToContext(ctx context.Context, _ *thehive.APIClient, creds *TheHiveCredentials) context.Context {
	if creds.APIKey != "" && strings.ToLower(creds.APIKey) != dummyAPIKey {
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

// ExtractBearerToken strips a "Bearer " prefix if present, else returns the header unchanged.
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

// ValidateTheHiveClient verifies the client's credentials by calling TheHive's
// current-user endpoint, returning an error if the call fails, the status is
// not 200, or no user info is returned.
func ValidateTheHiveClient(ctx context.Context, client *thehive.APIClient) error {
	currentUser, resp, err := client.UserAPI.GetCurrentUserInfo(ctx).Execute()
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if err != nil {
		return fmt.Errorf("failed to validate TheHive credentials: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to validate TheHive credentials: unexpected status code %d", resp.StatusCode)
	}

	if currentUser == nil {
		return errors.New("failed to validate TheHive credentials: current user info is nil")
	}

	return nil
}

// validateTheHiveAuthInContext validates the client in ctx and records the
// outcome: AuthValidatedCtxKey on success, AuthErrorCtxKey otherwise. A non-nil
// cache skips the upstream call for recently validated creds; failures aren't cached.
func validateTheHiveAuthInContext(ctx context.Context, creds *TheHiveCredentials, cache *validationCache) context.Context {
	client, ok := ctx.Value(types.HiveClientCtxKey).(*thehive.APIClient)
	if !ok || client == nil {
		return ctx
	}

	if cache != nil && cache.IsValid(creds) {
		return context.WithValue(ctx, types.AuthValidatedCtxKey, true)
	}

	err := ValidateTheHiveClient(ctx, client)
	if err != nil {
		slog.Error("TheHive authentication failed", "error", err)
		return context.WithValue(ctx, types.AuthErrorCtxKey, fmt.Errorf("TheHive authentication failed: %w", err))
	}

	slog.Info("TheHive authentication validated successfully")

	if cache != nil {
		cache.MarkValid(creds)
	}

	return context.WithValue(ctx, types.AuthValidatedCtxKey, true)
}

// SafeGetEnv gets an environment variable with optional default value
func SafeGetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return defaultValue
}

// LoadPermissions loads permissions config. "admin" grants full permissions,
// "read_only" or "" defaults to read-only; any other value is a file path.
func LoadPermissions(configPath string) (*permissions.Config, error) {
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
