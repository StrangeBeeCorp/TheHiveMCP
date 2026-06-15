package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// WritePermissionsFile writes a permissions YAML document to a temporary file
// and returns its path, for use with GetMCPTestClientWithPermissions.
func WritePermissionsFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "permissions.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
