package testutils

import (
	"context"
	"log"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// NewTestClient starts (or reuses) the integration TheHive instance and returns
// an API client scoped to the main test organisation.
func NewTestClient(t *testing.T) *thehive.APIClient {
	t.Helper()

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

// SetupTestWithCleanup returns a test client and registers a t.Cleanup that
// resets the hive instance.
func SetupTestWithCleanup(t *testing.T) *thehive.APIClient {
	t.Helper()

	client := NewTestClient(t)

	t.Cleanup(func() {
		url, err := StartTheHiveContainer(t)
		if err != nil {
			t.Logf("Warning: Failed to get container URL for cleanup: %v", err)
			return
		}

		testConfig := NewHiveTestConfig()

		err = ResetHiveInstance(t, url, testConfig)
		if err != nil {
			t.Logf("Warning: Failed to reset hive instance: %v", err)
		}
	})

	return client
}
