package bootstrap

import "errors"

// errUnexpectedHandlerCall is returned by test tool handlers that must never be
// reached; it lets those handlers avoid returning (nil, nil).
var errUnexpectedHandlerCall = errors.New("tool handler must not be called")

// Shared test-only string constants, extracted to satisfy goconst.
const (
	testHiveExampleURL = "https://thehive.example.com"
	testHiveComURL     = "https://thehive.com"
	testOtherHiveURL   = "https://other.thehive.com"
	testHiveInternal   = "http://hive.internal:9000"
	testDefaultHiveURL = "https://default.thehive.com"

	testOrg      = "my-org"
	testValidKey = "valid-key"

	// #nosec G101 -- test fixture credential value, not a real secret
	testEnvSecretKey = "env-secret-key"
)
