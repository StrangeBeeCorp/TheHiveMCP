package testutils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// WritePermissionsFile writes content to a temp file and returns its path, for
// use with GetMCPTestClientWithPermissions.
func WritePermissionsFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "permissions.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// PermissionsFixture returns the absolute path to a fixture under
// docs/examples/permissions/. Anchored on this source file (not the working
// directory) so it survives test files moving between packages.
//
// Example: PermissionsFixture(t, "analyst.yaml")
func PermissionsFixture(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "PermissionsFixture: unable to resolve caller source path")

	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..") // internal/testutils/ -> repo root
	path := filepath.Join(repoRoot, "docs", "examples", "permissions", name)

	require.FileExists(t, path, "permission fixture %q not found at %s", name, path)

	return path
}
