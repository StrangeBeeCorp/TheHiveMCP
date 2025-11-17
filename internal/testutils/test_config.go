package testutils

import "github.com/StrangeBeeCorp/TheHiveMCP/internal/types"

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
		ImageName:     "strangebee/thehive:5.5.2",
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
		LogLevel:              "DEBUG",
		OpenAIBaseURL:         "",
		OpenAIAPIKey:          "",
		OpenAIModel:           "",
		OpenAIMaxTokens:       0,
	}
}
