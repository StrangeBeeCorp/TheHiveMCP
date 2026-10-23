package testutils

import (
	"context"
	"log"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// NewTestClient starts (or reuses) the integration TheHive instance and returns
// an API client scoped to this test's dedicated organisation, authenticated as
// this test's dedicated user (see orgs.go).
func NewTestClient(t *testing.T) *thehive.APIClient {
	t.Helper()

	url, err := StartTheHiveContainer(t)
	if err != nil {
		log.Fatalf("Failed to start container: %v", err)
	}

	env := testEnvFor(t)
	cfg := &Config{
		URL:      url,
		Username: env.username,
		Password: env.password,
		APIKey:   env.apiKey,
		OrgName:  env.org,
	}

	return CreateOrgClient(t, cfg)
}

// NewAdminClient returns a client of the admin organisation and the superadmin's
// auth context, for the instance-wide settings a test must create (custom
// fields). Never in shared mode: the shared TheHive's settings belong to its
// stack, not to a test.
func NewAdminClient(t *testing.T) (*thehive.APIClient, context.Context) {
	t.Helper()

	if SharedMode() {
		t.Fatal("NewAdminClient: the shared TheHive's instance-wide settings belong to its stack")
	}

	url, err := StartTheHiveContainer(t)
	if err != nil {
		log.Fatalf("Failed to start container: %v", err)
	}

	return CreateOrgClient(t, &Config{URL: url, OrgName: adminOrg}), CreateAuthContext(adminUser(), adminPassword())
}

// GetAuthContext returns an auth context (basic auth, or the API key in shared
// mode) for this test's dedicated user, so
// data seeded through it lands in the test's own organisation. The org itself
// comes from the client's X-Organisation header, not this context.
func GetAuthContext(t *testing.T) context.Context {
	t.Helper()

	env := testEnvFor(t)

	return env.authContext()
}

// SetupTestWithCleanup returns a test client scoped to this test's organisation
// (see orgs.go). In license mode each test owns a throwaway org that is simply
// abandoned; in free-license mode the shared org is purged on t.Cleanup so the
// next sequential test starts clean. Either way the caller needs no teardown.
func SetupTestWithCleanup(t *testing.T) *thehive.APIClient {
	t.Helper()

	return NewTestClient(t)
}
