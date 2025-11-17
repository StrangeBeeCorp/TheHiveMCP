package testutils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	globalContainer testcontainers.Container
	globalPort      string
)

type Config struct {
	URL      string
	Username string
	Password string
	OrgName  string
}

// StartTheHiveContainer starts a TheHive container for testing
func StartTheHiveContainer(t *testing.T) (string, error) {
	t.Helper()

	if globalContainer != nil {
		return fmt.Sprintf("http://localhost:%s", globalPort), nil
	}

	req := testcontainers.ContainerRequest{
		Image:        "strangebee/thehive:5.5.2",
		ExposedPorts: []string{"9000/tcp"},
		WaitingFor:   wait.ForHTTP("/api/status").WithPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(t.Context(), testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return "", err
	}

	port, err := container.MappedPort(t.Context(), "9000")
	if err != nil {
		return "", err
	}

	globalContainer = container
	globalPort = port.Port()
	url := fmt.Sprintf("http://localhost:%s", globalPort)

	if err := initHiveInstance(t, url); err != nil {
		return "", fmt.Errorf("failed to initialize hive instance: %w", err)
	}

	return url, nil
}

// CreateOrgClient creates a client configured for a specific organization
func CreateOrgClient(t *testing.T, cfg *Config) *thehive.APIClient {
	if t != nil {
		t.Helper()
	}

	clientCfg := thehive.NewConfiguration()
	clientCfg.Host = strings.TrimPrefix(strings.TrimPrefix(cfg.URL, "http://"), "https://")
	clientCfg.Scheme = "http"
	if strings.HasPrefix(cfg.URL, "https://") {
		clientCfg.Scheme = "https"
	}

	clientCfg.AddDefaultHeader("X-Organisation", cfg.OrgName)
	return thehive.NewAPIClient(clientCfg)
}

// CreateAuthContext creates an authentication context for API calls
func CreateAuthContext(username, password string) context.Context {
	auth := thehive.BasicAuth{
		UserName: username,
		Password: password,
	}
	return context.WithValue(context.Background(), thehive.ContextBasicAuth, auth)
}

// ResetHiveInstance clears all data from the test organizations
func ResetHiveInstance(t *testing.T, hiveUrl string, testConfig *HiveTestConfig) error {
	t.Helper()

	for _, org := range []string{testConfig.MainOrg, testConfig.AdminOrg} {
		if err := resetOrganization(t, hiveUrl, org); err != nil {
			return err
		}
	}
	return nil
}

// Internal helpers
func initHiveInstance(t *testing.T, url string) error {
	adminConfig := &Config{
		URL:      url,
		Username: "admin@thehive.local",
		Password: "secret",
		OrgName:  "admin",
	}

	client, ctx := createClientAndContext(t, adminConfig)
	testConfig := NewHiveTestConfig()

	ensureTestOrganization(t, client, ctx, testConfig.MainOrg)
	setupUserPermissions(t, client, ctx, testConfig.MainOrg)

	return nil
}

func createClientAndContext(t *testing.T, cfg *Config) (*thehive.APIClient, context.Context) {
	if t != nil {
		t.Helper()
	}

	var client *thehive.APIClient
	if cfg.URL != "" {
		client = CreateOrgClient(t, cfg)
	}

	auth := thehive.BasicAuth{
		UserName: cfg.Username,
		Password: cfg.Password,
	}

	baseCtx := context.Background()

	return client, context.WithValue(baseCtx, thehive.ContextBasicAuth, auth)
}

func ensureTestOrganization(t *testing.T, client *thehive.APIClient, ctx context.Context, orgName string) string {
	t.Helper()

	// Check if org exists
	genericOp := thehive.NewInputQueryGenericOperation("listOrganisation")
	query := thehive.NewInputQuery()
	query.SetQuery([]thehive.InputQueryNamedOperation{
		thehive.InputQueryGenericOperationAsInputQueryNamedOperation(genericOp),
	})

	resp, httpResp, err := client.QueryAndExportAPI.QueryAPI(ctx).InputQuery(*query).Execute()
	if err == nil && httpResp.StatusCode == 200 && resp != nil {
		var orgs []thehive.OutputOrganisation
		if jsonBytes, _ := json.Marshal(resp); jsonBytes != nil {
			if json.Unmarshal(jsonBytes, &orgs) == nil {
				for _, org := range orgs {
					if org.GetName() == orgName {
						return org.GetUnderscoreId()
					}
				}
			}
		}
	}

	// Create if doesn't exist
	createOrgInput := thehive.NewInputCreateOrganisation(orgName, "Integration test organization")
	createResp, httpResp, err := client.OrganisationAPI.CreateOrganisation(ctx).
		InputCreateOrganisation(*createOrgInput).Execute()

	if err != nil && httpResp != nil && (httpResp.StatusCode == 409 || httpResp.StatusCode == 403) {
		return orgName
	}
	require.NoError(t, err)
	require.Equal(t, 201, httpResp.StatusCode)

	return createResp.GetUnderscoreId()
}

func setupUserPermissions(t *testing.T, client *thehive.APIClient, ctx context.Context, orgName string) {
	t.Helper()

	userResp, httpResp, err := client.UserAPI.GetCurrentUserInfo(ctx).Execute()
	if err != nil || httpResp.StatusCode != http.StatusOK || userResp == nil {
		t.Fatalf("Could not get current user info: %v", err)
	}

	userID := userResp.GetUnderscoreId()
	orgAssignments := []thehive.InputUserOrganisation{
		{Organisation: orgName, Profile: "org-admin"},
		{Organisation: "admin", Profile: "admin"},
	}

	updateInput := thehive.NewInputSetUserOrganisations()
	updateInput.SetOrganisations(orgAssignments)

	_, httpResp, err = client.UserAPI.SetUserOrganisations(ctx, userID).
		InputSetUserOrganisations(*updateInput).Execute()

	if err != nil || httpResp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to set user organizations: %v, status: %d", err, httpResp.StatusCode)
	}
}

func resetOrganization(t *testing.T, hiveUrl string, org string) error {
	t.Helper()

	cfg := &Config{
		URL:      hiveUrl,
		Username: "admin@thehive.local",
		Password: "secret",
		OrgName:  org,
	}

	client, ctx := createClientAndContext(t, cfg)

	// Delete all entities in order
	entityTypes := []struct {
		name      string
		operation string
		deleteAPI func(*thehive.APIClient, context.Context, string) (*http.Response, error)
	}{
		{"alerts", "listAlert", func(c *thehive.APIClient, ctx context.Context, id string) (*http.Response, error) {
			return c.AlertAPI.DeleteAlert(ctx, id).Execute()
		}},
		{"cases", "listCase", func(c *thehive.APIClient, ctx context.Context, id string) (*http.Response, error) {
			return c.CaseAPI.DeleteCase(ctx, id).Execute()
		}},
		{"case templates", "listCaseTemplate", func(c *thehive.APIClient, ctx context.Context, id string) (*http.Response, error) {
			return c.CaseTemplateAPI.DeleteCaseTemplate(ctx, id).Execute()
		}},
		{"tasks", "listTask", func(c *thehive.APIClient, ctx context.Context, id string) (*http.Response, error) {
			return c.TaskAPI.DeleteTask(ctx, id).Execute()
		}},
	}

	for _, entity := range entityTypes {
		if err := deleteAllEntities(t, client, ctx, entity.name, entity.operation, entity.deleteAPI); err != nil {
			return err
		}
	}

	return nil
}

func deleteAllEntities(
	t *testing.T,
	client *thehive.APIClient,
	ctx context.Context,
	entityName string,
	listOperation string,
	deleteFunc func(*thehive.APIClient, context.Context, string) (*http.Response, error),
) error {
	t.Helper()

	// Query for entities
	query := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(
				thehive.NewInputQueryGenericOperation(listOperation),
			),
		},
	}

	resp, _, err := client.QueryAndExportAPI.QueryAPI(ctx).InputQuery(query).Execute()
	if err != nil {
		return fmt.Errorf("error listing %s: %w", entityName, err)
	}

	// Parse response
	respBytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var entities []map[string]interface{}
	if err := json.Unmarshal(respBytes, &entities); err != nil {
		return fmt.Errorf("error parsing %s: %w", entityName, err)
	}

	// Delete each entity
	for _, entity := range entities {
		id, ok := entity["_id"].(string)
		if !ok {
			continue
		}

		if _, err := deleteFunc(client, ctx, id); err != nil {
			return fmt.Errorf("error deleting %s %s: %w", entityName, id, err)
		}
	}

	return nil
}
