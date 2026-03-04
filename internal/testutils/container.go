package testutils

import (
	"bytes"
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
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMITREPatternID is the patternId available in the test TheHive instance after initHiveInstance
const TestMITREPatternID = "T1059"

// minimalSTIXBundle is a minimal MITRE ATT&CK STIX 2.0 bundle for testing
const minimalSTIXBundle = `{
  "type": "bundle",
  "id": "bundle--test-attck-bundle",
  "spec_version": "2.0",
  "objects": [
    {
      "type": "attack-pattern",
      "id": "attack-pattern--7385dfaf-6886-4229-9ecd-6fd678040830",
      "created": "2017-05-31T21:31:43.540Z",
      "modified": "2023-01-01T00:00:00.000Z",
      "name": "Command and Scripting Interpreter",
      "description": "Adversaries may abuse command and script interpreters to execute commands.",
      "kill_chain_phases": [
        {
          "kill_chain_name": "mitre-attack",
          "phase_name": "execution"
        }
      ],
      "external_references": [
        {
          "source_name": "mitre-attack",
          "external_id": "T1059",
          "url": "https://attack.mitre.org/techniques/T1059"
        }
      ],
      "x_mitre_platforms": ["Windows", "macOS", "Linux"],
      "x_mitre_is_subtechnique": false,
      "x_mitre_version": "2.1"
    }
  ]
}`

var (
	globalContainer testcontainers.Container
	globalNetwork   *testcontainers.DockerNetwork
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
		WaitingFor:   wait.ForHTTP("/api/status").WithPort("9000/tcp").WithStartupTimeout(5 * time.Minute),
	}

	if globalNetwork == nil {
		nw, err := network.New(t.Context())
		if err != nil {
			return "", fmt.Errorf("failed to create test network: %w", err)
		}
		globalNetwork = nw
	}
	req.Networks = []string{globalNetwork.Name}

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

// CreateOrgClient creates a client configured for a specific organisation
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

// ResetHiveInstance clears all data from the test organisations
func ResetHiveInstance(t *testing.T, hiveUrl string, testConfig *HiveTestConfig) error {
	t.Helper()

	for _, org := range []string{testConfig.MainOrg, testConfig.AdminOrg} {
		if err := resetOrganisation(t, hiveUrl, org); err != nil {
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

	ensureTestOrganisation(t, client, ctx, testConfig.MainOrg)
	setupUserPermissions(t, client, ctx, testConfig.MainOrg)

	if err := setupAttackPatterns(ctx, client); err != nil {
		return fmt.Errorf("failed to setup ATT&CK patterns: %w", err)
	}

	return nil
}

// setupAttackPatterns imports a minimal MITRE ATT&CK pattern catalog into TheHive.
// It serves the STIX bundle from a sidecar container on the same Docker network as TheHive.
func setupAttackPatterns(ctx context.Context, client *thehive.APIClient) error {
	if globalNetwork == nil {
		return fmt.Errorf("test network not initialized")
	}

	const mitreServerAlias = "mitre-server"

	mitreRequest := testcontainers.ContainerRequest{
		Image:        "nginx:alpine",
		ExposedPorts: []string{"80/tcp"},
		Networks:     []string{globalNetwork.Name},
		NetworkAliases: map[string][]string{
			globalNetwork.Name: {mitreServerAlias},
		},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            bytes.NewBufferString(minimalSTIXBundle),
				ContainerFilePath: "/usr/share/nginx/html/mitre.json",
				FileMode:          0o644,
			},
		},
		WaitingFor: wait.ForListeningPort("80/tcp").WithStartupTimeout(30 * time.Second),
	}

	mitreContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: mitreRequest,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("failed to start MITRE sidecar container: %w", err)
	}
	defer func() { _ = mitreContainer.Terminate(ctx) }()

	url := fmt.Sprintf("http://%s:80/mitre.json", mitreServerAlias)

	input := thehive.NewInputPatternImportMitre("mitre-attack")
	input.SetUrl(url)

	_, _, err = client.AttckAPI.ImportMITREAttckFile(ctx).InputPatternImportMitre(*input).Execute()
	return err
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

func ensureTestOrganisation(t *testing.T, client *thehive.APIClient, ctx context.Context, orgName string) string {
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
	createOrgInput := thehive.NewInputCreateOrganisation(orgName, "Integration test organisation")
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
		t.Fatalf("Failed to set user organisations: %v, status: %d", err, httpResp.StatusCode)
	}
}

func resetOrganisation(t *testing.T, hiveUrl string, org string) error {
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
