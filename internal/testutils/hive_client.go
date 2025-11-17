package testutils

import (
	"context"
	"log"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func NewTestClient(t *testing.T) *thehive.APIClient {
	url, err := StartTheHiveContainer(t)
	if err != nil {
		log.Fatalf("Failed to start container: %v", err)
	}

	testConfig := NewHiveTestConfig()
	cfg := &Config{
		URL:      url,
		Username: testConfig.User,
		Password: testConfig.Password,
		OrgName:  testConfig.MainOrg,
	}

	return CreateOrgClient(t, cfg)
}

// GetAuthContext creates a context with authentication for API calls
func GetAuthContext(testConfig *HiveTestConfig) context.Context {
	return CreateAuthContext(testConfig.User, testConfig.Password)
}

// GetAdminAuthContext creates a context with admin authentication for API calls
func GetAdminAuthContext(testConfig *HiveTestConfig) context.Context {
	return CreateAuthContext(testConfig.User, testConfig.Password)
}

// SetupTestWithCleanup creates a test client and registers automatic cleanup
// Call this at the beginning of each test function
func SetupTestWithCleanup(t *testing.T) *thehive.APIClient {
	client := NewTestClient(t)

	// Register cleanup function that will run after the test completes
	t.Cleanup(func() {
		url, err := StartTheHiveContainer(t)
		if err != nil {
			t.Logf("Warning: Failed to get container URL for cleanup: %v", err)
			return
		}

		testConfig := NewHiveTestConfig()
		if err := ResetHiveInstance(t, url, testConfig); err != nil {
			t.Logf("Warning: Failed to reset hive instance: %v", err)
		}
	})

	return client
}
