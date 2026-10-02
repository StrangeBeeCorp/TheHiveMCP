// Package gittest builds scratch git repositories for the harness's tests. The tests run git
// itself: make test runs them in an image that has it, so a missing git fails a test rather than
// skipping it.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	dirPerm  = 0o750
	filePerm = 0o600
)

// Repo is a new repository on branch main, with files written and committed: its root.
func Repo(t *testing.T, files map[string]string) string {
	t.Helper()

	_, err := exec.LookPath("git")
	require.NoError(t, err, "git is needed: run the tests through make test, whose image has it")

	dir := t.TempDir()
	Git(t, dir, "init", "-q", "-b", "main")
	Write(t, dir, files)
	Git(t, dir, "add", "-A")
	Commit(t, dir)

	return dir
}

// Write writes files under dir, creating their directories.
func Write(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), dirPerm))
		require.NoError(t, os.WriteFile(p, []byte(content), filePerm))
	}
}

// Git runs git in dir with the test's arguments, away from the user's and the system's
// configuration, and returns its output.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...) // #nosec G204 -- the test's own git arguments
	cmd.Dir = dir

	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)

	return strings.TrimSpace(string(out))
}

// Commit commits what is staged, unsigned and without hooks.
func Commit(t *testing.T, dir string) {
	t.Helper()
	Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false",
		"-c", "core.hooksPath=/dev/null", "commit", "-q", "--allow-empty", "-m", "x")
}
