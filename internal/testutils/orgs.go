package testutils

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// The integration suite gives every test its own freshly-created TheHive
// organisation AND a dedicated user that is org-admin on it. Tests are thus
// fully isolated and can run in parallel. TheHive has no DeleteOrganisation
// API, and `compose down -v` wipes the whole stack after the suite, so per-test
// orgs/users are simply abandoned — zero teardown cost.
//
// Why a dedicated per-test user (not the shared admin): the only grant endpoint
// (SetUserOrganisations) *replaces* a user's whole org list. Repeatedly calling
// it on the one shared admin from many parallel tests churns that user's
// auth/permission state and 401s other tests' in-flight requests. Creating an
// independent user per org (a single CreateUser call, no shared-state mutation)
// removes the contention entirely.
//
// The per-test credentials reach TheHive via basic-auth (CreateAuthContext) and
// the org via the X-Organisation header (CreateOrgClient / MCP creds). Both the
// raw API client and the MCP client resolve the same cell for a given
// *testing.T, so they agree with no signature changes at the call sites.
var (
	// orgByTest maps a *testing.T to its lazily-provisioned per-test cell.
	orgByTest sync.Map
	// orgCounter makes org/user names unique without time/rand (deterministic,
	// -race-safe, and usable regardless of environment).
	orgCounter atomic.Int64
	// sharedUserOnce creates the free-mode shared user exactly once across the
	// whole (sequential) suite; createOrgUser already treats a 409 as success,
	// so this is belt-and-suspenders against redundant CreateUser calls.
	sharedUserOnce sync.Once
	// createUserMu serialises CreateUser calls across parallel tests. TheHive's
	// handler is not concurrency-safe: two overlapping calls make it fail with
	//
	//	java.lang.IllegalStateException: Sink.asPublisher(fanout = false) only
	//	supports one subscriber
	//
	// which surfaced as a flaky 500 during provisioning.
	//
	// It guards one attempt, never the retry loop around it: holding a global
	// lock across a backoff sleep would serialise the whole suite behind the
	// slowest provision, which is far worse than the flake it fixes.
	createUserMu sync.Mutex
)

// Parallel marks a test as safe to run concurrently, but ONLY in license mode.
//
// In license mode (THEHIVE_TEST_LICENSE set) each test gets its own org + user,
// so it calls t.Parallel() as usual. In free-license mode all tests share
// main-org and must run sequentially: Parallel is a no-op there, so the test
// runs in source order and its t.Cleanup (purgeOrg) completes before the next
// test starts — the ordering the shared-org data reset depends on.
//
// Integration tests call testutils.Parallel(t) as their first statement in
// place of t.Parallel(). Pure unit tests (no live TheHive) keep t.Parallel().
func Parallel(t *testing.T) {
	t.Helper()

	if LicensePresent() {
		t.Parallel()
	}
}

// testEnv is one test's isolated org + dedicated user. once ensures the org and
// user are created exactly once even if both helpers enter concurrently for the
// same t.
type testEnv struct {
	once     sync.Once
	org      string
	username string
	password string
	err      error
}

// testEnvFor returns the per-test isolated environment, provisioning the org +
// dedicated user on first call for this t. Both NewTestClient and
// GetMCPTestClientWithPermissions call it, so they agree on one env per test.
func testEnvFor(t *testing.T) *testEnv {
	t.Helper()

	cellAny, _ := orgByTest.LoadOrStore(t, &testEnv{})

	cell, ok := cellAny.(*testEnv)
	if !ok {
		t.Fatalf("unexpected test env cell type %T", cellAny)
	}

	cell.once.Do(func() {
		cell.err = provisionTestEnv(t, cell)
		t.Cleanup(func() { orgByTest.Delete(t) })

		// Free-license mode reuses one shared org across all (sequential) tests,
		// so each test must purge the data it created; otherwise the next test's
		// absolute-count assertions (require.Len / require.Equal on row counts)
		// would see leftovers. License mode gives each test its own throwaway org
		// and abandons it — zero teardown.
		if cell.err == nil && !LicensePresent() {
			t.Cleanup(func() { purgeOrg(t, cell) })
		}
	})

	if cell.err != nil {
		t.Fatalf("failed to provision per-test org/user: %v", cell.err)
	}

	return cell
}

// provisionTestEnv creates a uniquely-named org and a dedicated org-admin user
// in it, using the bootstrap admin. It mutates no shared identity, so it needs
// no cross-test locking beyond the per-cell once.
func provisionTestEnv(t *testing.T, cell *testEnv) error {
	t.Helper()

	url, err := StartTheHiveContainer(t)
	if err != nil {
		return err
	}

	cell.password = testAdminPassword

	adminConfig := &Config{
		URL:      url,
		Username: DefaultAdminUser,
		Password: testAdminPassword,
		OrgName:  adminOrg,
	}
	client, ctx := createClientAndContext(t, adminConfig)

	if !LicensePresent() {
		// Free-license mode: TheHive's built-in license caps organisations at 1,
		// so every test shares the already-bootstrapped main-org and one shared
		// org-admin user. Tests run sequentially (see Parallel) and purge their
		// own data on cleanup, so sharing the org is safe.
		cell.org = NewHiveTestConfig().MainOrg
		cell.username = sharedFreeUser

		sharedUserOnce.Do(func() {
			createOrgUser(ctx, t, client, cell.org, cell.username, cell.password)
		})

		return nil
	}

	n := orgCounter.Add(1)
	base := sanitizeOrgName(t.Name())
	cell.org = fmt.Sprintf("%s-%d", base, n)
	// TheHive logins must look like emails; the local part must be unique.
	cell.username = fmt.Sprintf("%s-%d@test.local", base, n)

	ensureTestOrganisation(ctx, t, client, cell.org)
	createOrgUser(ctx, t, client, cell.org, cell.username, cell.password)

	return nil
}

// TestUserLogin returns the login of this test's dedicated user. Use it where a
// test asserts on or assigns to the acting user's identity, instead of the
// former shared admin login (the admin is not a member of the per-test org).
func TestUserLogin(t *testing.T) string {
	t.Helper()

	return testEnvFor(t).username
}

// sanitizeOrgName maps a test name to a TheHive-safe org name: lowercase, only
// [a-z0-9-], subtest '/' separators and '_' folded to '-', and truncated so the
// counter suffix keeps the whole name within TheHive's length limit.
func sanitizeOrgName(name string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '/' || r == '_' || r == '-' || r == ' ':
			b.WriteRune('-')
		}
	}

	sanitized := strings.Trim(b.String(), "-")
	if sanitized == "" {
		sanitized = "test"
	}

	const maxBase = 40
	if len(sanitized) > maxBase {
		sanitized = strings.Trim(sanitized[:maxBase], "-")
	}

	return sanitized
}

// createUserRetryBudget bounds the retry loop in createOrgUser. It mirrors the
// readiness budget ensureOrganisation already applies to organisation creation:
// the same freshly-booted TheHive that 5xxs on one also 5xxs on the other.
const createUserRetryBudget = 2 * time.Minute

// createOrgUser creates a dedicated org-admin user directly in orgName with the
// given password, in a single CreateUser call. A 409 (already exists, e.g. a
// retried provision) is treated as success.
//
// Transient 5xx responses are retried until createUserRetryBudget expires,
// matching ensureOrganisation: a TheHive that answers /api/status is not yet
// necessarily done migrating, and it returns 500 until it is. Each attempt
// takes createUserMu; the waiting between attempts does not.
func createOrgUser(ctx context.Context, t *testing.T, client *thehive.APIClient, orgName, login, password string) {
	t.Helper()

	deadline := time.Now().Add(createUserRetryBudget)

	for {
		status, err := tryCreateOrgUser(ctx, client, orgName, login, password)
		if err == nil {
			return
		}

		// 5xx only: a 4xx is a genuine bad request and will never succeed.
		if status < 500 || time.Now().After(deadline) {
			t.Fatalf("Failed to create org-admin user %q in %q: %v, status: %d", login, orgName, err, status)
		}

		time.Sleep(3 * time.Second)
	}
}

// tryCreateOrgUser performs one CreateUser attempt, returning the HTTP status
// so the caller can decide whether it is worth retrying. A 409 counts as
// success: the user already exists, which is all the caller needs.
func tryCreateOrgUser(ctx context.Context, client *thehive.APIClient, orgName, login, password string) (int, error) {
	createUserMu.Lock()
	defer createUserMu.Unlock()

	input := thehive.NewInputCreateUser(login, login, "org-admin")
	input.SetPassword(password)
	input.SetOrganisation(orgName)

	_, httpResp, err := client.UserAPI.CreateUser(ctx).InputCreateUser(*input).Execute()
	closeResponse(httpResp)

	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}

	if err == nil || status == http.StatusConflict {
		return status, nil
	}

	body := ""

	var apiErr *thehive.GenericOpenAPIError
	if errors.As(err, &apiErr) {
		body = string(apiErr.Body())
	}

	return status, fmt.Errorf("create user %q in %q: %w (body: %s)", login, orgName, err, body)
}
