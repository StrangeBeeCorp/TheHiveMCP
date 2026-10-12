package gitx_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const aFile = "a.txt"

// read is the content of a file the test wrote.
func read(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path) // #nosec G304 -- a file of the test's own repository
	require.NoError(t, err)

	return string(data)
}

// A scratch checkout holds the revision, takes a patch, and leaves no trace once removed; the
// working tree it came from is never touched.
func TestAScratchCheckoutTakesAPatchAndGoesAway(t *testing.T) {
	t.Parallel()

	dir := gittest.Repo(t, map[string]string{aFile: "one\n"})
	gittest.Write(t, dir, map[string]string{aFile: "uncommitted\n"})

	scratch, remove, err := gitx.Scratch(t.Context(), dir, "HEAD")
	require.NoError(t, err)
	assert.Equal(t, "one\n", read(t, filepath.Join(scratch, aFile)), "the revision, not the working tree")

	patch := filepath.Join(t.TempDir(), "p.patch")
	require.NoError(t, os.WriteFile(patch, []byte("diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-one\n+two\n"), 0o600))
	require.NoError(t, gitx.Apply(t.Context(), scratch, patch))

	assert.Equal(t, "two\n", read(t, filepath.Join(scratch, aFile)))
	require.Error(t, gitx.Apply(t.Context(), scratch, patch), "a patch that no longer applies")

	remove()
	assert.NoDirExists(t, scratch)
	assert.Len(t, strings.Split(strings.TrimSpace(gittest.Git(t, dir, "worktree", "list")), "\n"), 1)

	assert.Equal(t, "uncommitted\n", read(t, filepath.Join(dir, aFile)))
}

// HasPath and At read a file at a revision, and say when it has none.
func TestAFileIsReadAtARevision(t *testing.T) {
	t.Parallel()

	dir := gittest.Repo(t, map[string]string{aFile: "one\n"})
	gittest.Write(t, dir, map[string]string{aFile: "two\n", "b.txt": "new\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)

	assert.True(t, gitx.HasPath(t.Context(), dir, "HEAD~1", aFile))
	assert.False(t, gitx.HasPath(t.Context(), dir, "HEAD~1", "b.txt"))
	assert.True(t, gitx.HasPath(t.Context(), dir, "HEAD", "b.txt"))

	data, ok := gitx.At(t.Context(), dir, "HEAD~1", aFile)
	require.True(t, ok)
	assert.Equal(t, "one\n", string(data))

	_, ok = gitx.At(t.Context(), dir, "HEAD~1", "b.txt")
	assert.False(t, ok)
}
