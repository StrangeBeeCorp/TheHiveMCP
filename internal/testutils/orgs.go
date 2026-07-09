package testutils

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

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
)

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

	n := orgCounter.Add(1)
	base := sanitizeOrgName(t.Name())
	cell.org = fmt.Sprintf("%s-%d", base, n)
	// TheHive logins must look like emails; the local part must be unique.
	cell.username = fmt.Sprintf("%s-%d@test.local", base, n)
	cell.password = testAdminPassword

	adminConfig := &Config{
		URL:      url,
		Username: DefaultAdminUser,
		Password: testAdminPassword,
		OrgName:  adminOrg,
	}
	client, ctx := createClientAndContext(t, adminConfig)

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

// createOrgUser creates a dedicated org-admin user directly in orgName with the
// given password, in a single CreateUser call. A 409 (already exists, e.g. a
// retried provision) is treated as success. Uses the bootstrap admin client;
// mutates no shared user, so it is safe to run concurrently across tests.
func createOrgUser(ctx context.Context, t *testing.T, client *thehive.APIClient, orgName, login, password string) {
	t.Helper()

	input := thehive.NewInputCreateUser(login, login, "org-admin")
	input.SetPassword(password)
	input.SetOrganisation(orgName)

	_, httpResp, err := client.UserAPI.CreateUser(ctx).InputCreateUser(*input).Execute()
	closeResponse(httpResp)

	if err == nil {
		return
	}

	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}

	if status == 409 {
		// Already created (e.g. provision retry) — fine for test setup.
		return
	}

	body := ""

	var apiErr *thehive.GenericOpenAPIError
	if errors.As(err, &apiErr) {
		body = string(apiErr.Body())
	}

	t.Fatalf("Failed to create org-admin user %q in %q: %v, status: %d, body: %s", login, orgName, err, status, body)
}
