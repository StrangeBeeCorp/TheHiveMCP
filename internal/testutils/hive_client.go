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

func GetAuthContext(testConfig *HiveTestConfig) context.Context {
	return CreateAuthContext(testConfig.User, testConfig.Password)
}

func GetAdminAuthContext(testConfig *HiveTestConfig) context.Context {
	return CreateAuthContext(testConfig.User, testConfig.Password)
}

// SetupTestWithCleanup returns a test client and registers a t.Cleanup that
// resets the hive instance.
func SetupTestWithCleanup(t *testing.T) *thehive.APIClient {
	client := NewTestClient(t)

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
