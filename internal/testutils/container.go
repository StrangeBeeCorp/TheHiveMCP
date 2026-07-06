// Package testutils provides helpers for the integration test suite: the
// docker-compose-managed TheHive bootstrap, API/MCP test clients, and fixtures.
package testutils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"
)

// TestMITREPatternID is the patternId available in the test TheHive instance after initHiveInstance
const TestMITREPatternID = "T1059"

// defaultTheHiveTestURL is the published TheHive port from docker-compose.test.yml.
const defaultTheHiveTestURL = "http://localhost:9000"

// statusReadinessTimeout bounds how long we wait for TheHive to answer
// /api/status after `docker compose up`. A cold boot of the heavier versions
// takes several minutes, so this is generous; org setup then has its own
// (shorter) retry window for the post-status migration phase.
const statusReadinessTimeout = 8 * time.Minute

const (
	// testAdminPassword is the default password of the built-in TheHive
	// superadmin in the integration stack. #nosec G101 -- test fixture, not a real secret
	testAdminPassword = "secret"
	// adminOrg is the built-in TheHive administration organisation / profile name.
	adminOrg = "admin"
	// testTag is the shared tag/type/source literal used by the mock fixtures.
	testTag = "test"
)

var (
	initOnce sync.Once
	errInit  error
	hiveURL  string
)

// TheHiveTestURL returns the base URL of the compose-managed TheHive instance,
// overridable with THEHIVE_TEST_URL (e.g. when the stack runs on another host).
func TheHiveTestURL() string {
	if u := os.Getenv("THEHIVE_TEST_URL"); u != "" {
		return u
	}

	return defaultTheHiveTestURL
}

// Config holds the connection settings for a single-organisation TheHive API
// client used by the integration helpers.
type Config struct {
	URL      string
	Username string
	Password string
	OrgName  string
}

// StartTheHiveContainer returns the URL of the TheHive instance the integration
// suite runs against. The TheHive + Elasticsearch + MITRE stack itself is
// managed by docker compose (see docker-compose.test.yml), brought up by
// `make test`; this helper waits for it to become ready and performs the
// one-time org/permission/ATT&CK bootstrap.
//
// Tests relying on a real TheHive instance are integration tests: they are slow
// and cannot be result-cached by `go test`. Running in `-short` mode skips them
// so a `-short` run stays fast and fully cacheable.
func StartTheHiveContainer(t *testing.T) (string, error) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test requiring a TheHive instance (-short)")
	}

	initOnce.Do(func() {
		hiveURL = TheHiveTestURL()

		err := waitForStatus(hiveURL, statusReadinessTimeout)
		if err != nil {
			errInit = err
			return
		}

		errInit = initHiveInstance(t, hiveURL)
	})

	if errInit != nil {
		return "", fmt.Errorf("failed to initialize hive instance at %s: %w", hiveURL, errInit)
	}

	return hiveURL, nil
}

// closeResponse closes an HTTP response body if the response is non-nil. The
// thehive4go SDK returns the raw *http.Response alongside the decoded payload;
// closing it here keeps the connection reusable and satisfies bodyclose.
func closeResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// waitForStatus polls TheHive's /api/status until it returns 200 or the timeout
// elapses. It probes over HTTP from the test process itself, so it needs no
// in-container tooling (the TheHive/ES images ship no guaranteed HTTP client).
func waitForStatus(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 10 * time.Second}
	deadline := time.Now().Add(timeout)

	var lastErr error

	for {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url+"/api/status", http.NoBody)
		if err != nil {
			return fmt.Errorf("build status request for %s: %w", url, err)
		}

		resp, err := client.Do(req)
		switch {
		case err != nil:
			lastErr = err
		case resp.StatusCode == http.StatusOK:
			_ = resp.Body.Close()
			return nil
		default:
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("TheHive at %s not ready within %s: %w", url, timeout, lastErr)
		}

		time.Sleep(3 * time.Second)
	}
}

// CreateOrgClient creates a client configured for a specific organisation
func CreateOrgClient(t *testing.T, cfg *Config) *thehive.APIClient {
	t.Helper()

	clientCfg := thehive.NewConfiguration()
	clientCfg.Host = strings.TrimPrefix(strings.TrimPrefix(cfg.URL, "http://"), "https://")

	clientCfg.Scheme = "http"
	if strings.HasPrefix(cfg.URL, "https://") {
		clientCfg.Scheme = "https"
	}

	// Cap every request so a server-side stall fails that one call in bounded
	// time instead of hanging until the package timeout. Under memory pressure
	// (the heavier 5.6.3 stack on a 16 GB CI runner) TheHive can stop answering
	// a request mid-flight; without this, a single stalled call took down the
	// whole internal/tools package (see DL-6007). 90s is far above a healthy
	// round-trip, so it never trips a slow-but-live server.
	clientCfg.HTTPClient = &http.Client{Timeout: 90 * time.Second}

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

// TeardownContainers is retained as the TestMain cleanup hook but is now a
// no-op: the TheHive + Elasticsearch + MITRE stack is owned by docker compose
// (see docker-compose.test.yml) and torn down by `make test`, not per test
// binary. Kept so existing TestMain bodies compile unchanged:
//
//	func TestMain(m *testing.M) {
//	    code := m.Run()
//	    testutils.TeardownContainers(context.Background())
//	    os.Exit(code)
//	}
func TeardownContainers(_ context.Context) {
	// Intentionally empty: docker compose owns the stack lifecycle (`make test`
	// runs `compose down`), so there is nothing to tear down per test binary.
}

// ResetHiveInstance clears all data from the test organisations
func ResetHiveInstance(t *testing.T, hiveURL string, testConfig *HiveTestConfig) error {
	t.Helper()

	for _, org := range []string{testConfig.MainOrg, testConfig.AdminOrg} {
		err := resetOrganisation(t, hiveURL, org)
		if err != nil {
			return err
		}
	}

	return nil
}

// Internal helpers
func initHiveInstance(t *testing.T, url string) error {
	t.Helper()

	adminConfig := &Config{
		URL:      url,
		Username: DefaultAdminUser,
		Password: testAdminPassword,
		OrgName:  adminOrg,
	}

	client, ctx := createClientAndContext(t, adminConfig)
	testConfig := NewHiveTestConfig()

	ensureTestOrganisation(ctx, t, client, testConfig.MainOrg)
	setupUserPermissions(ctx, t, client, testConfig.MainOrg)

	err := setupAttackPatterns(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to setup ATT&CK patterns: %w", err)
	}

	return nil
}

// mitreServerURL is the compose `mitre-server` nginx sidecar serving the
// minimal STIX bundle (internal/testutils/testdata/mitre.json). TheHive fetches
// it server-side over the compose network, so the URL is the service name.
const mitreServerURL = "http://mitre-server/mitre.json"

// setupAttackPatterns imports a minimal MITRE ATT&CK pattern catalog into
// TheHive from the compose-managed `mitre-server` sidecar.
func setupAttackPatterns(ctx context.Context, client *thehive.APIClient) error {
	input := thehive.NewInputPatternImportMitre("mitre-attack")
	input.SetUrl(mitreServerURL)

	_, httpResp, err := client.AttckAPI.ImportMITREAttckFile(ctx).InputPatternImportMitre(*input).Execute()
	closeResponse(httpResp)

	if err != nil {
		return fmt.Errorf("failed to import MITRE ATT&CK patterns from %s: %w", mitreServerURL, err)
	}

	return nil
}

func createClientAndContext(t *testing.T, cfg *Config) (*thehive.APIClient, context.Context) {
	t.Helper()

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

func ensureTestOrganisation(ctx context.Context, t *testing.T, client *thehive.APIClient, orgName string) string {
	t.Helper()

	// TheHive answers the /api/status readiness probe (used by the container wait
	// strategy) before its schema migration completes, so the first organisation
	// setup can transiently fail with 5xx — especially when several containers
	// boot in parallel. Retry those, but fail fast on non-retriable errors.
	const readinessTimeout = 4 * time.Minute

	deadline := time.Now().Add(readinessTimeout)

	for {
		id, retry, err := tryEnsureTestOrganisation(ctx, client, orgName)
		if err == nil {
			return id
		}

		if !retry {
			t.Fatalf("organisation %q setup failed: %v", orgName, err)
		}

		if time.Now().After(deadline) {
			t.Fatalf("TheHive not ready to create organisation %q within %s: %v", orgName, readinessTimeout, err)
		}

		time.Sleep(3 * time.Second)
	}
}

// findOrganisationID returns the id of orgName within a listOrganisation query
// response, or false if it is absent or the response cannot be decoded.
func findOrganisationID(resp any, orgName string) (string, bool) {
	jsonBytes, err := json.Marshal(resp)
	if err != nil || jsonBytes == nil {
		return "", false
	}

	var orgs []thehive.OutputOrganisation
	if json.Unmarshal(jsonBytes, &orgs) != nil {
		return "", false
	}

	for _, org := range orgs {
		if org.GetName() == orgName {
			return org.GetUnderscoreId(), true
		}
	}

	return "", false
}

// tryEnsureTestOrganisation performs one lookup-or-create attempt for orgName.
// It returns err == nil once the organisation exists (or already existed). On
// failure, retry is true for transient startup errors (5xx / transport failure)
// that the caller should wait out, and false for non-retriable errors (e.g. a
// 4xx misconfiguration) that should fail fast.
func tryEnsureTestOrganisation(ctx context.Context, client *thehive.APIClient, orgName string) (id string, retry bool, err error) {
	// Check if org exists
	genericOp := thehive.NewInputQueryGenericOperation("listOrganisation")
	query := thehive.NewInputQuery()
	query.SetQuery([]thehive.InputQueryNamedOperation{
		thehive.InputQueryGenericOperationAsInputQueryNamedOperation(genericOp),
	})

	resp, httpResp, listErr := client.QueryAndExportAPI.QueryAPI(ctx).InputQuery(*query).Execute()
	closeResponse(httpResp)

	if listErr == nil && httpResp != nil && httpResp.StatusCode == http.StatusOK && resp != nil {
		if id, found := findOrganisationID(resp, orgName); found {
			return id, false, nil
		}
	}

	// Create if it doesn't exist
	createOrgInput := thehive.NewInputCreateOrganisation(orgName, "Integration test organisation")

	createResp, httpResp, createErr := client.OrganisationAPI.CreateOrganisation(ctx).
		InputCreateOrganisation(*createOrgInput).Execute()
	closeResponse(httpResp)

	if createErr == nil && httpResp != nil && httpResp.StatusCode == http.StatusCreated {
		return createResp.GetUnderscoreId(), false, nil
	}

	if httpResp != nil && (httpResp.StatusCode == http.StatusConflict || httpResp.StatusCode == http.StatusForbidden) {
		// Already exists / created concurrently — good enough for test setup.
		return orgName, false, nil
	}

	// Retry only while TheHive is still starting: a 5xx, or not yet reachable
	// (transport error / no response). Other statuses (4xx) are not retriable.
	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}

	transient := httpResp == nil || status >= 500
	if createErr != nil {
		return "", transient, fmt.Errorf("create organisation %q (status %d): %w", orgName, status, createErr)
	}

	return "", transient, fmt.Errorf("create organisation %q: unexpected status %d", orgName, status)
}

func setupUserPermissions(ctx context.Context, t *testing.T, client *thehive.APIClient, orgName string) {
	t.Helper()

	userResp, httpResp, err := client.UserAPI.GetCurrentUserInfo(ctx).Execute()
	closeResponse(httpResp)

	if err != nil || httpResp.StatusCode != http.StatusOK || userResp == nil {
		t.Fatalf("Could not get current user info: %v", err)
	}

	userID := userResp.GetUnderscoreId()
	orgAssignments := []thehive.InputUserOrganisation{
		{Organisation: orgName, Profile: "org-admin"},
		{Organisation: adminOrg, Profile: adminOrg},
	}

	updateInput := thehive.NewInputSetUserOrganisations()
	updateInput.SetOrganisations(orgAssignments)

	_, httpResp, err = client.UserAPI.SetUserOrganisations(ctx, userID).
		InputSetUserOrganisations(*updateInput).Execute()
	closeResponse(httpResp)

	if err != nil || httpResp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to set user organisations: %v, status: %d", err, httpResp.StatusCode)
	}
}

func resetOrganisation(t *testing.T, hiveURL string, org string) error {
	t.Helper()

	cfg := &Config{
		URL:      hiveURL,
		Username: DefaultAdminUser,
		Password: testAdminPassword,
		OrgName:  org,
	}

	client, ctx := createClientAndContext(t, cfg)

	// Delete all entities in order
	entityTypes := []struct {
		name      string
		operation string
		deleteAPI func(context.Context, *thehive.APIClient, string) (*http.Response, error)
	}{
		{"alerts", "listAlert", func(ctx context.Context, c *thehive.APIClient, id string) (*http.Response, error) {
			return c.AlertAPI.DeleteAlert(ctx, id).Execute()
		}},
		{"cases", "listCase", func(ctx context.Context, c *thehive.APIClient, id string) (*http.Response, error) {
			return c.CaseAPI.DeleteCase(ctx, id).Execute()
		}},
		{"case templates", "listCaseTemplate", func(ctx context.Context, c *thehive.APIClient, id string) (*http.Response, error) {
			return c.CaseTemplateAPI.DeleteCaseTemplate(ctx, id).Execute()
		}},
		{"tasks", "listTask", func(ctx context.Context, c *thehive.APIClient, id string) (*http.Response, error) {
			return c.TaskAPI.DeleteTask(ctx, id).Execute()
		}},
	}

	for _, entity := range entityTypes {
		err := deleteAllEntities(ctx, t, client, entity.name, entity.operation, entity.deleteAPI)
		if err != nil {
			return err
		}
	}

	return nil
}

func deleteAllEntities(
	ctx context.Context,
	t *testing.T,
	client *thehive.APIClient,
	entityName string,
	listOperation string,
	deleteFunc func(context.Context, *thehive.APIClient, string) (*http.Response, error),
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

	resp, httpResp, err := client.QueryAndExportAPI.QueryAPI(ctx).InputQuery(query).Execute()
	closeResponse(httpResp)

	if err != nil {
		return fmt.Errorf("error listing %s: %w", entityName, err)
	}

	// Parse response
	respBytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var entities []map[string]any

	err = json.Unmarshal(respBytes, &entities)
	if err != nil {
		return fmt.Errorf("error parsing %s: %w", entityName, err)
	}

	// Delete each entity. A 404 means the entity is already gone — typically
	// because deleting a parent case cascade-deleted its tasks before this loop
	// reaches them. That is the desired end state, so tolerate it rather than
	// aborting the reset and leaking the remaining entities into the next test.
	for _, entity := range entities {
		id, ok := entity["_id"].(string)
		if !ok {
			continue
		}

		resp, err := deleteFunc(ctx, client, id)
		closeResponse(resp)

		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}

			return fmt.Errorf("error deleting %s %s: %w", entityName, id, err)
		}
	}

	return nil
}
