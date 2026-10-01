package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// A permissions file the server cannot load must stop it at startup. Past
// startup, a failed load only logs a warning, and every tool call is then
// denied with no word on why.
func TestCheckPermissionsConfigRefusesABadFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "permissions.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
      filter: {}
`), 0o600))

	require.ErrorContains(t, checkPermissionsConfig(&types.TheHiveMcpDefaultOptions{PermissionsConfigPath: path}), "invalid permissions configuration")
	require.NoError(t, checkPermissionsConfig(&types.TheHiveMcpDefaultOptions{}), "the default profile loads")

	// The address cannot be bound, so a StartHTTPServer that skipped the check
	// would fail on the bind instead of serving forever and hanging the test.
	err := StartHTTPServer(GetMCPServer(), &types.TheHiveMcpDefaultOptions{
		BindAddr: "256.0.0.1:0", TheHiveURL: testHiveExampleURL, PermissionsConfigPath: path,
	})
	require.ErrorContains(t, err, "invalid permissions configuration")
}
