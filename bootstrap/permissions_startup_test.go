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
func TestServersRefuseToStartOnABadPermissionsFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "permissions.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
      filter: {}
`), 0o600))

	options := &types.TheHiveMcpDefaultOptions{
		BindAddr:              "127.0.0.1:0",
		TheHiveURL:            "https://thehive.example.com",
		PermissionsConfigPath: path,
	}

	require.ErrorContains(t, StartHTTPServer(GetMCPServer(), options), "invalid permissions configuration")
	require.ErrorContains(t, StartStdioServer(GetMCPServer(), options), "invalid permissions configuration")
}
