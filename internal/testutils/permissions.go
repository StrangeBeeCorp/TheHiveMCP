package testutils

import (
	"os"
	"path/filepath"
	"runtime"
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

// PermissionsFixture returns the absolute path to a permission example fixture
// under docs/examples/permissions/, independent of the test's working directory.
//
// It anchors on this source file's location (internal/testutils/) so it keeps
// working if test files move between package directories. The fixture must
// exist on disk; a missing fixture fails loudly here instead of surfacing later
// as a confusing "permissions file" load error.
//
// Example: PermissionsFixture(t, "analyst.yaml")
func PermissionsFixture(t *testing.T, name string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "PermissionsFixture: unable to resolve caller source path")

	// thisFile = <repo>/internal/testutils/permissions.go
	// repo root = dir(thisFile)/../..
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "docs", "examples", "permissions", name)

	require.FileExists(t, path, "permission fixture %q not found at %s", name, path)
	return path
}
