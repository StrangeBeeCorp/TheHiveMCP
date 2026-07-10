package testutils

import (
	"os"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// DefaultTheHiveTestImage is used when THEHIVE_TEST_IMAGE is unset (CI sets it
// per version-matrix leg). Renovate tracks this tag via the custom Docker
// manager in renovate.json.
const DefaultTheHiveTestImage = "strangebee/thehive:5.6.3"

// DefaultAdminUser is the built-in TheHive superadmin login used across the
// integration suite for authentication and as a default assignee in fixtures.
const DefaultAdminUser = "admin@thehive.local"

// sharedFreeUser is the single org-admin login used by the free-license
// (no-THEHIVE_TEST_LICENSE) path, where every test shares main-org and runs
// sequentially. In license mode each test instead gets its own unique user
// (see provisionTestEnv in orgs.go).
const sharedFreeUser = "shared@test.local"

// LicensePresent reports whether the suite is running in license mode, keyed on
// THEHIVE_TEST_LICENSE being non-empty:
//   - present  ⇒ per-test org + user, tests run in parallel (multi-org).
//   - absent   ⇒ one shared main-org, tests run sequentially and purge their
//     own data on cleanup (free-license path; the public-CI default).
//
// The variable is NOT a user input: the harness (scripts/reset-integration-db.sh
// + the Makefile) decides the mode by whether the StrangeBee licensing image is
// pullable, mints a dev license on the fly when it is, and injects the minted
// token here for the go-test container. So this reads that harness-set signal.
func LicensePresent() bool {
	return os.Getenv("THEHIVE_TEST_LICENSE") != ""
}

// TheHiveTestImage returns the integration-suite image: THEHIVE_TEST_IMAGE
// overrides the default (how the CI matrix selects 5.5 vs 5.6).
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
		User:          DefaultAdminUser,
		Password:      testAdminPassword,
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
		TheHiveUsername:       DefaultAdminUser,
		TheHivePassword:       testAdminPassword,
		TheHiveOrganisation:   "main-org",
		MCPServerEndpointPath: "/mcp",
		MCPHeartbeatInterval:  "30s",
		TransportType:         "inprocess",
		BindAddr:              "",
		LogLevel:              testLogLevel(),
	}
}
