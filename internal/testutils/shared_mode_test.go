package testutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Shared mode is opt-in: the default (variable unset or anything but a truthy
// value) must keep the disposable compose-stack behaviour.
func TestSharedModeIsOptIn(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"0":     false,
		"false": false,
		"no":    false,
		"1":     true,
		"true":  true,
		"TRUE":  true,
		"yes":   true,
		"on":    true,
	}

	for value, want := range cases {
		t.Run("THEHIVE_TEST_SHARED="+value, func(t *testing.T) {
			t.Setenv(envTestShared, value)
			assert.Equal(t, want, SharedMode())
		})
	}
}

// Shared mode shares one organisation across every test, so it must never run
// tests in parallel, even when a licence is also exported.
func TestLicensePresentIsFalseInSharedMode(t *testing.T) {
	t.Setenv("THEHIVE_TEST_LICENSE", "token")
	t.Setenv(envTestShared, "")
	assert.True(t, LicensePresent())

	t.Setenv(envTestShared, "1")
	assert.False(t, LicensePresent())
}

// The compose stack's superadmin defaults stay as they were; the environment
// only overrides them.
func TestAdminLoginDefaultsAndOverrides(t *testing.T) {
	t.Setenv(envTestAdminUser, "")
	t.Setenv(envTestAdminPassword, "")
	assert.Equal(t, "admin@thehive.local", adminUser())
	assert.Equal(t, testAdminPassword, adminPassword())

	cfg := NewHiveTestConfig()
	assert.Equal(t, "admin@thehive.local", cfg.User)
	assert.Equal(t, testAdminPassword, cfg.Password)

	t.Setenv(envTestAdminUser, "root@example.test")
	t.Setenv(envTestAdminPassword, "changed-pw")
	assert.Equal(t, "root@example.test", adminUser())
	assert.Equal(t, "changed-pw", adminPassword())
	assert.Equal(t, "root@example.test", NewHiveTestConfig().User)
}
