package testutils

import (
	"os"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// DefaultTheHiveTestImage is used when THEHIVE_TEST_IMAGE is unset (CI sets it
// per version-matrix leg). Renovate tracks this tag via the custom Docker
// manager in renovate.json.
const DefaultTheHiveTestImage = "strangebee/thehive:5.6.3"

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

type HiveTestConfig struct {
	ImageName     string
	ContainerName string
	User          string
	Password      string
	MainOrg       string
	AdminOrg      string
}

func NewHiveTestConfig() *HiveTestConfig {
	return &HiveTestConfig{
		ImageName:     TheHiveTestImage(),
		ContainerName: "thehive4go-integration-tester",
		User:          "admin@thehive.local",
		Password:      "secret",
		MainOrg:       "main-org",
		AdminOrg:      "admin",
	}
}

func NewMCPTestConfig() *types.TheHiveMcpDefaultOptions {
	return &types.TheHiveMcpDefaultOptions{
		TheHiveURL:            "http://localhost:9000",
		TheHiveAPIKey:         "",
		TheHiveUsername:       "admin@thehive.local",
		TheHivePassword:       "secret",
		TheHiveOrganisation:   "main-org",
		MCPServerEndpointPath: "/mcp",
		MCPHeartbeatInterval:  "30s",
		TransportType:         "inprocess",
		BindAddr:              "",
		LogLevel:              testLogLevel(),
	}
}
