package testutils

import (
	"os"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// DefaultTheHiveTestImage is used when THEHIVE_TEST_IMAGE is unset (CI sets it
// per version-matrix leg). Renovate tracks this tag via the custom Docker
// manager in renovate.json.
const DefaultTheHiveTestImage = "strangebee/thehive:5.8.0"

// DefaultAdminUser is the built-in TheHive superadmin login used across the
// integration suite for authentication and as a default assignee in fixtures.
const DefaultAdminUser = "admin@thehive.local"

// sharedFreeUser is the single org-admin login used by the free-license
// (no-THEHIVE_TEST_LICENSE) path, where every test shares main-org and runs
// sequentially. In license mode each test instead gets its own unique user
// (see provisionTestEnv in orgs.go).
const sharedFreeUser = "shared@test.local"

// Environment variables of the opt-in shared mode (see SharedMode).
const (
	envTestShared = "THEHIVE_TEST_SHARED"
	envTestOrg    = "THEHIVE_TEST_ORG"
	envTestAPIKey = "THEHIVE_TEST_API_KEY" // #nosec G101 -- env var name, not a real credential
)

// Environment variables that override the built-in superadmin of the compose
// stack (own-stack mode only; shared mode never uses the platform admin).
const (
	envTestAdminUser     = "THEHIVE_TEST_ADMIN_USER"
	envTestAdminPassword = "THEHIVE_TEST_ADMIN_PASSWORD" // #nosec G101 -- env var name, not a real credential
)

// SharedMode reports whether the suite runs inside ONE organisation of an
// already-running, shared TheHive instead of the disposable compose stack
// (opt-in: THEHIVE_TEST_SHARED=1, see `make test-integration-shared`).
//
// In shared mode the suite never uses the platform admin and never changes
// anything instance-wide: it authenticates with the API key of an org-admin of
// THEHIVE_TEST_ORG, runs the tests sequentially in that organisation, and
// purges the organisation after every test (see purgeOrg).
func SharedMode() bool {
	switch strings.ToLower(os.Getenv(envTestShared)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// sharedOrg and sharedAPIKey are the shared-mode target; only meaningful when
// SharedMode is true.
func sharedOrg() string { return os.Getenv(envTestOrg) }

func sharedAPIKey() string { return os.Getenv(envTestAPIKey) }

// adminUser is the superadmin login of the compose stack: THEHIVE_TEST_ADMIN_USER
// overrides the built-in default (DefaultAdminUser).
func adminUser() string {
	if u := os.Getenv(envTestAdminUser); u != "" {
		return u
	}

	return DefaultAdminUser
}

// adminPassword is the superadmin password of the compose stack:
// THEHIVE_TEST_ADMIN_PASSWORD overrides the built-in default (testAdminPassword).
func adminPassword() string {
	if p := os.Getenv(envTestAdminPassword); p != "" {
		return p
	}

	return testAdminPassword
}

// LicensePresent reports whether the suite is running in license mode, keyed on
// THEHIVE_TEST_LICENSE being non-empty:
//   - present  ⇒ per-test org + user, tests run in parallel (multi-org).
//   - absent   ⇒ one shared main-org, tests run sequentially and purge their
//     own data on cleanup (free-license path; the public-CI default).
//
// The variable is NOT a user input: the harness (bin/harness integration (internal/harness/stack)
// + the Makefile) decides the mode by whether the StrangeBee licensing image is
// pullable, mints a dev license on the fly when it is, and injects the minted
// token here for the go-test container. So this reads that harness-set signal.
//
// It is always false in shared mode: the single organisation is shared by every
// test, so the tests run sequentially whatever licence the instance has.
func LicensePresent() bool {
	return !SharedMode() && os.Getenv("THEHIVE_TEST_LICENSE") != ""
}

// TheHiveTestImage returns the integration-suite image: THEHIVE_TEST_IMAGE
// overrides the default (how the CI matrix selects 5.5 vs 5.8).
func TheHiveTestImage() string {
	if img := os.Getenv("THEHIVE_TEST_IMAGE"); img != "" {
		return img
	}

	return DefaultTheHiveTestImage
}

// testLogLevel: warn by default so a failing test shows the assertion, not
// pages of request logs. Set LOG_LEVEL=debug (or info) for the full trace.
func testLogLevel() string {
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		return lvl
	}

	return "warn"
}

// HiveTestConfig holds the credentials and organisation names used by the
// integration suite against the compose-managed TheHive instance.
type HiveTestConfig struct {
	ImageName     string
	ContainerName string
	User          string
	Password      string
	MainOrg       string
	AdminOrg      string
}

// NewHiveTestConfig returns the default HiveTestConfig for the integration suite.
//
// MainOrg / AdminOrg are the bootstrap seed orgs only. Per-test scoping no
// longer flows from here: each test gets its own freshly-created org via
// testOrgName (see orgs.go), which the client/MCP-creds constructors read
// directly. The org field here is kept for the one-time boot provisioning.
func NewHiveTestConfig() *HiveTestConfig {
	return &HiveTestConfig{
		ImageName:     TheHiveTestImage(),
		ContainerName: "thehive4go-integration-tester",
		User:          adminUser(),
		Password:      adminPassword(),
		MainOrg:       "main-org",
		AdminOrg:      adminOrg,
	}
}

// NewMCPTestConfig returns the default in-process MCP server options for tests.
// TheHiveOrganisation here is a placeholder default; GetMCPTestClient* overrides
// it with the per-test org from testOrgName (see orgs.go).
func NewMCPTestConfig() *types.TheHiveMcpDefaultOptions {
	return &types.TheHiveMcpDefaultOptions{
		TheHiveURL:            "http://localhost:9000",
		TheHiveAPIKey:         "",
		TheHiveUsername:       adminUser(),
		TheHivePassword:       adminPassword(),
		TheHiveOrganisation:   "main-org",
		MCPServerEndpointPath: "/mcp",
		MCPHeartbeatInterval:  "30s",
		TransportType:         "inprocess",
		BindAddr:              "",
		LogLevel:              testLogLevel(),
	}
}
