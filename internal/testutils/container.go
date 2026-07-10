// Package testutils provides helpers for the integration test suite: the
// docker-compose-managed TheHive bootstrap, API/MCP test clients, and fixtures.
package testutils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// TestMITREPatternID is the patternId available in the test TheHive instance after initHiveInstance
const TestMITREPatternID = "T1059"

// defaultTheHiveTestURL: published port from docker-compose.test.yml.
const defaultTheHiveTestURL = "http://localhost:9000"

// statusReadinessTimeout: cold boot of heavier versions takes several minutes.
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

// StartTheHiveContainer returns the URL of the compose-managed TheHive instance
// (see docker-compose.test.yml, brought up by `make test`), after waiting for
// readiness and performing the one-time org/permission/ATT&CK bootstrap.
// Skips under `-short`, since these integration tests are slow and uncacheable.
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

// waitForStatus probes /api/status over HTTP from the test process, so it needs
// no in-container tooling (the images ship no guaranteed HTTP client).
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

	// Per-request cap: a mid-flight stall (TheHive under memory pressure) once
	// hung the whole internal/tools package until the package timeout (DL-6007).
	// 90s is well above a healthy round-trip, so it never trips a live server.
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

// TeardownContainers is a no-op kept so existing TestMain bodies compile:
// docker compose owns the stack lifecycle (`make test` runs `compose down`).
func TeardownContainers(_ context.Context) {
	// Intentionally empty: docker compose owns teardown; nothing to do here.
}

// initHiveInstance runs the one-time boot: MITRE ATT&CK import (global catalog
// shared by all orgs) plus the default main-org provisioning. It runs inside
// StartTheHiveContainer's initOnce, so the t that wins the race may not own the
// failure — it MUST return errors (propagated via errInit), never call t.Fatalf.
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

	_, err := ensureOrganisation(ctx, client, testConfig.MainOrg)
	if err != nil {
		return fmt.Errorf("failed to ensure organisation %q: %w", testConfig.MainOrg, err)
	}

	err = setupAttackPatterns(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to setup ATT&CK patterns: %w", err)
	}

	return nil
}

// mitreServerURL uses the compose service name because TheHive fetches it
// server-side over the compose network (sidecar serves testdata/mitre.json).
const mitreServerURL = "http://mitre-server/mitre.json"

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

// ensureOrganisation looks up or creates orgName, retrying transient startup
// errors. It returns an error rather than calling t.Fatalf so it is safe both on
// the initOnce boot path (where the caller doesn't own the failing test) and on
// the per-test path via ensureTestOrganisation.
func ensureOrganisation(ctx context.Context, client *thehive.APIClient, orgName string) (string, error) {
	// /api/status returns 200 before schema migration completes, so early org
	// setup can transiently 5xx (worse when several containers boot in parallel).
	// Retry those; fail fast on non-retriable errors.
	const readinessTimeout = 4 * time.Minute

	deadline := time.Now().Add(readinessTimeout)

	for {
		id, retry, err := tryEnsureTestOrganisation(ctx, client, orgName)
		if err == nil {
			return id, nil
		}

		if !retry {
			return "", fmt.Errorf("organisation %q setup failed: %w", orgName, err)
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("TheHive not ready to create organisation %q within %s: %w", orgName, readinessTimeout, err)
		}

		time.Sleep(3 * time.Second)
	}
}

// ensureTestOrganisation is the per-test wrapper around ensureOrganisation. It
// runs outside initOnce (in provisionTestOrg), so it owns its own t and may
// fail the test directly.
func ensureTestOrganisation(ctx context.Context, t *testing.T, client *thehive.APIClient, orgName string) string {
	t.Helper()

	id, err := ensureOrganisation(ctx, client, orgName)
	if err != nil {
		t.Fatalf("%v", err)
	}

	return id
}

// purgeOrgTimeout bounds the delete-until-empty loop in purgeOrg. Two effects
// make a single delete sweep insufficient: the server's list is capped at a
// page window (so a heavy test may need several sweeps to drain), and
// Elasticsearch refreshes its index asynchronously (so a listX right after a
// delete can still return just-deleted rows). purgeOrg re-lists and re-deletes
// until a sweep sees nothing, so the next sequential test starts clean.
const purgeOrgTimeout = 30 * time.Second

// purgeOrg deletes every entity the shared free-mode org accumulated during a
// test, re-sweeping until a full pass finds nothing left. It runs only in
// free-license mode (see testEnvFor), where all tests share one org and run
// sequentially, so a clean slate between tests is what keeps the suite's
// absolute-count assertions valid.
//
// It acts as the test's dedicated user, scoped to the org via the X-Organisation
// header — the same credentials/scope the test itself used. MITRE ATT&CK
// patterns are a global catalog shared by all orgs and are never deleted.
func purgeOrg(t *testing.T, cell *testEnv) {
	t.Helper()

	cfg := &Config{
		URL:      hiveURL,
		Username: cell.username,
		Password: cell.password,
		OrgName:  cell.org,
	}
	client, ctx := createClientAndContext(t, cfg)

	deadline := time.Now().Add(purgeOrgTimeout)

	for {
		remaining := purgeSweep(ctx, t, client, cell.org)
		if remaining == 0 {
			return
		}

		if time.Now().After(deadline) {
			t.Logf("purgeOrg: %q still had %d entities after %s; next test may see leftovers", cell.org, remaining, purgeOrgTimeout)
			return
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// purgeSweep runs one delete pass over the org's entities and returns how many
// it saw (before deleting). A return of 0 means the org listed empty — the
// loop's exit condition. Deletes are best-effort and merely logged on failure:
// a transient error just leaves rows for the next sweep to retry.
func purgeSweep(ctx context.Context, t *testing.T, client *thehive.APIClient, orgName string) int {
	t.Helper()

	seen := 0

	// Delete cases first: DeleteCase cascades a case's tasks, observables, pages,
	// procedures and comments, so afterwards listPage returns only standalone
	// org pages.
	caseIDs := listEntityIDs(ctx, t, client, "listCase")
	seen += len(caseIDs)

	for _, id := range caseIDs {
		resp, err := client.CaseAPI.DeleteCase(ctx, id).Execute()
		closeResponse(resp)

		if err != nil {
			t.Logf("purgeOrg: delete case %s in %q: %v", id, orgName, err)
		}
	}

	if alertIDs := listEntityIDs(ctx, t, client, "listAlert"); len(alertIDs) > 0 {
		seen += len(alertIDs)

		body := thehive.NewDeleteAlertInBulkRequest(alertIDs)
		resp, err := client.AlertAPI.DeleteAlertInBulk(ctx).DeleteAlertInBulkRequest(*body).Execute()
		closeResponse(resp)

		if err != nil {
			t.Logf("purgeOrg: bulk-delete alerts in %q: %v", orgName, err)
		}
	}

	caseTemplateIDs := listEntityIDs(ctx, t, client, "listCaseTemplate")
	seen += len(caseTemplateIDs)

	for _, id := range caseTemplateIDs {
		resp, err := client.CaseTemplateAPI.DeleteCaseTemplate(ctx, id).Execute()
		closeResponse(resp)

		if err != nil {
			t.Logf("purgeOrg: delete case template %s in %q: %v", id, orgName, err)
		}
	}

	pageIDs := listEntityIDs(ctx, t, client, "listPage")
	seen += len(pageIDs)

	for _, id := range pageIDs {
		resp, err := client.PageAPI.DeleteAPage(ctx, id).Execute()
		closeResponse(resp)

		if err != nil {
			t.Logf("purgeOrg: delete page %s in %q: %v", id, orgName, err)
		}
	}

	return seen
}

// purgeListPageSize bounds the page appended to every purge listing. TheHive's
// /api/v1/query returns only a capped default window when no `page` operation is
// present, so a bare listX could miss rows a heavier test created and leave them
// undeleted. We request an explicit wide page (matching what the production
// search handler does — see buildPagingOperation) so a single call sees every
// entity; purgeOrg still loops in case a test somehow exceeds even this.
const purgeListPageSize = 10000

// listEntityIDs runs a generic list operation (e.g. "listCase") in the current
// org context and returns the _id of every result, up to purgeListPageSize. It
// reuses the same QueryAPI generic-op pattern as tryEnsureTestOrganisation, plus
// an explicit wide `page` op so it is not truncated by the server's default page
// window. A failed query yields an empty slice (the caller retries), never a
// fatal error.
func listEntityIDs(ctx context.Context, t *testing.T, client *thehive.APIClient, operation string) []string {
	t.Helper()

	genericOp := thehive.NewInputQueryGenericOperation(operation)
	pageOp := thehive.NewInputQueryPagingOperation(0, purgeListPageSize, "page")
	query := thehive.NewInputQuery()
	query.SetQuery([]thehive.InputQueryNamedOperation{
		thehive.InputQueryGenericOperationAsInputQueryNamedOperation(genericOp),
		thehive.InputQueryPagingOperationAsInputQueryNamedOperation(pageOp),
	})

	resp, httpResp, err := client.QueryAndExportAPI.QueryAPI(ctx).InputQuery(*query).Execute()
	closeResponse(httpResp)

	if err != nil || httpResp == nil || httpResp.StatusCode != http.StatusOK || resp == nil {
		return nil
	}

	return extractIDs(resp)
}

// extractIDs pulls the "_id" of each object from a generic-op query response.
func extractIDs(resp any) []string {
	jsonBytes, err := json.Marshal(resp)
	if err != nil || jsonBytes == nil {
		return nil
	}

	var rows []struct {
		ID string `json:"_id"`
	}
	if json.Unmarshal(jsonBytes, &rows) != nil {
		return nil
	}

	ids := make([]string, 0, len(rows))

	for _, r := range rows {
		if r.ID != "" {
			ids = append(ids, r.ID)
		}
	}

	return ids
}

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

// tryEnsureTestOrganisation makes one lookup-or-create attempt. retry is true
// for transient startup errors (5xx / transport failure), false for 4xx.
func tryEnsureTestOrganisation(ctx context.Context, client *thehive.APIClient, orgName string) (id string, retry bool, err error) {
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

	createOrgInput := thehive.NewInputCreateOrganisation(orgName, "Integration test organisation")

	createResp, httpResp, createErr := client.OrganisationAPI.CreateOrganisation(ctx).
		InputCreateOrganisation(*createOrgInput).Execute()
	closeResponse(httpResp)

	if createErr == nil && httpResp != nil && httpResp.StatusCode == http.StatusCreated {
		return createResp.GetUnderscoreId(), false, nil
	}

	if httpResp != nil && httpResp.StatusCode == http.StatusConflict {
		// Already exists / created concurrently — good enough for test setup.
		return orgName, false, nil
	}

	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}

	transient := httpResp == nil || status >= 500

	if createErr != nil {
		body := ""

		var apiErr *thehive.GenericOpenAPIError
		if errors.As(createErr, &apiErr) {
			body = string(apiErr.Body())
		}

		return "", transient, fmt.Errorf("create organisation %q (status %d, body %s): %w", orgName, status, body, createErr)
	}

	return "", transient, fmt.Errorf("create organisation %q: unexpected status %d", orgName, status)
}
