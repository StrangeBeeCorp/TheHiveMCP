package stack_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/stack"
)

func TestLicensingTagFollowsTheTheHiveImage(t *testing.T) {
	t.Parallel()

	for image, want := range map[string]string{
		"strangebee/thehive:5.6.3":                 "release-5.6",
		"strangebee/thehive:5.5.2":                 "release-5.5",
		"registry:5000/strangebee/thehive:5.7.0-1": "release-5.7",
		"strangebee/thehive:latest":                "",
		"strangebee/thehive":                       "",
		"":                                         "",
	} {
		assert.Equal(t, want, stack.LicensingTag(image), image)
	}
}

func TestTokenIsTheMintersTokenLine(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "eyJhbGciOi.x.y", stack.Token("Connecting to TheHive\nToken: eyJhbGciOi.x.y\r\nSetting license ... OK\n"))
	assert.Empty(t, stack.Token("Error: 401 Unauthorized\n"))
}

// The minter signs in as TheHive's --dev admin, whose credentials go in the URL, not in a literal.
func TestMintArgvMintsAMultiInstanceDevLicense(t *testing.T) {
	t.Parallel()

	argv, err := stack.MintArgv("ghcr.io/strangebee/tools/licensing:release-5.6", "http://localhost:9000", "admin", "p@ss")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"docker", "run", "--network", "host", "--rm", "-i", "ghcr.io/strangebee/tools/licensing:release-5.6",
		"--key-id", "dev", "--thehive", "http://admin:p%40ss@localhost:9000", "--expiration", "365days", "--customer", "StrangeBee",
		"--plan", "Platinum", "--multiInstance", "--quotas", "users.normal=-1,organisations=-1", "--stdout",
	}, argv)

	_, err = stack.MintArgv("img", "http://[::1", "admin", "secret")
	require.Error(t, err)
}
