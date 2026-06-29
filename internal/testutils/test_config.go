package testutils

import (
	"os"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// DefaultTheHiveTestImage is the TheHive container image booted by the
// integration suite when THEHIVE_TEST_IMAGE is unset. CI sets THEHIVE_TEST_IMAGE
// per version-matrix leg; locally this gives a sensible default.
//
// Renovate tracks this tag via the custom Docker manager in renovate.json.
const DefaultTheHiveTestImage = "strangebee/thehive:5.6.3"

// TheHiveTestImage returns the TheHive container image for the integration suite.
// It is the single source of truth for the image: THEHIVE_TEST_IMAGE overrides
// the default, which is how the CI matrix selects 5.5 vs 5.6.
func TheHiveTestImage() string {
	if img := os.Getenv("THEHIVE_TEST_IMAGE"); img != "" {
		return img
	}
	return DefaultTheHiveTestImage
}

// testLogLevel returns the slog level for the in-process test server. It is
// quiet (warn) by default so a failing test's output shows the assertion, not
// pages of INFO/HTTP request logs. Set LOG_LEVEL=debug (or info) to get the
// full request trace back when diagnosing a specific failure.
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
	// Some day we could test the OpenAI integration here too
	// but for now we keep it simple, only test with sampling handler
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
		OpenAIBaseURL:         "",
		OpenAIAPIKey:          "",
		OpenAIModel:           "",
		OpenAIMaxTokens:       0,
	}
}
