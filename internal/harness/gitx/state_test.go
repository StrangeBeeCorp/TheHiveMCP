package gitx_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ignoreRules = ".gitignore"
	pkgSource   = "package x\n"
	pkgEdited   = "package x // edited\n"
	newGo       = "new.go"
)

// The fingerprint changes with what the gates read, and only with it: an edit, staged or not, a
// new untracked file, a commit; not an ignored file.
func TestTheFingerprintChangesWithWhatTheGatesRead(t *testing.T) {
	t.Parallel()

	dir := gittest.Repo(t, map[string]string{goFile: pkgSource, ignoreRules: ".cache/\n"})
	fingerprint := func() string {
		t.Helper()

		fp, err := gitx.Fingerprint(t.Context(), dir)
		require.NoError(t, err)

		return fp
	}

	start := fingerprint()
	assert.Equal(t, start, fingerprint(), "nothing changed")

	gittest.Write(t, dir, map[string]string{".cache/harness/state.json": "{}"})
	assert.Equal(t, start, fingerprint(), "an ignored file is not read by the gates")

	gittest.Write(t, dir, map[string]string{goFile: pkgEdited})

	edited := fingerprint()
	assert.NotEqual(t, start, edited)

	gittest.Git(t, dir, "add", goFile)
	assert.Equal(t, edited, fingerprint(), "staging the same edit changes nothing the gates read")

	gittest.Write(t, dir, map[string]string{newGo: pkgSource})

	untracked := fingerprint()
	assert.NotEqual(t, edited, untracked)

	gittest.Write(t, dir, map[string]string{newGo: "package x // other\n"})
	assert.NotEqual(t, untracked, fingerprint(), "an untracked file's content counts")

	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)

	committed := fingerprint()

	gittest.Commit(t, dir)
	assert.NotEqual(t, committed, fingerprint(), "a new HEAD")
}

// Contents hash what a commit would carry and the ignored files of the directories named, never
// another ignored file or a deleted one; Dirty lists what HEAD does not hold as it is.
func TestContentsAndDirtyReadTheCheckout(t *testing.T) {
	t.Parallel()

	sealed := ".claude/sealed/DL-1/a_test.go"
	dir := gittest.Repo(t, map[string]string{goFile: pkgSource, ignoreRules: ".cache/\n.claude/sealed/\n", "gone.go": pkgSource})
	gittest.Write(t, dir, map[string]string{newGo: pkgSource, ".cache/x": "x\n", sealed: "x\n"})
	require.NoError(t, os.Remove(filepath.Join(dir, "gone.go")))

	files, err := gitx.Contents(t.Context(), dir, ".claude/sealed/")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{goFile, ignoreRules, newGo, sealed}, slices.Collect(maps.Keys(files)))
	assert.Equal(t, files[goFile], files[newGo], "the same content, the same hash")

	plain, err := gitx.Contents(t.Context(), dir)
	require.NoError(t, err)
	assert.NotContains(t, plain, sealed, "an ignored directory is read only when named")

	dirty, err := gitx.Dirty(t.Context(), dir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"gone.go", newGo}, dirty)
}

// Branch is empty on a detached HEAD, MainCheckout names the main working tree from a linked
// one, GitDir the linked one's own git files under the main .git (where its review state is),
// Ignored follows .gitignore, and Summary lists the changes and the last commits.
func TestTheCheckoutIsDescribed(t *testing.T) {
	t.Parallel()

	resolved := func(p string) string {
		r, err := filepath.EvalSymlinks(p)
		require.NoError(t, err)

		return r
	}

	dir := gittest.Repo(t, map[string]string{goFile: pkgSource, ignoreRules: ".cache/\n"})
	assert.Equal(t, "main", gitx.Branch(t.Context(), dir))
	assert.Equal(t, dir, filepath.Clean(gitx.MainCheckout(t.Context(), dir)))
	assert.Equal(t, resolved(filepath.Join(dir, ".git")), resolved(gitx.GitDir(t.Context(), dir)))

	linked := filepath.Join(t.TempDir(), "linked")
	gittest.Git(t, dir, "worktree", "add", "-q", "-b", "feat/x", linked)
	assert.Equal(t, "feat/x", gitx.Branch(t.Context(), linked))
	assert.Equal(t, filepath.Clean(dir), filepath.Clean(gitx.MainCheckout(t.Context(), linked)))
	assert.Equal(t, resolved(filepath.Join(dir, ".git", "worktrees", "linked")), resolved(gitx.GitDir(t.Context(), linked)),
		"a linked worktree's .git is a file: its git files are under the main checkout's")

	gittest.Git(t, dir, "checkout", "-q", "--detach")
	assert.Empty(t, gitx.Branch(t.Context(), dir))

	assert.True(t, gitx.Ignored(t.Context(), dir, ".cache/harness/BLOCKED.md"))
	assert.False(t, gitx.Ignored(t.Context(), dir, goFile))

	gittest.Write(t, dir, map[string]string{goFile: pkgEdited})
	summary := gitx.Summary(t.Context(), dir)
	assert.Contains(t, summary, "M "+goFile)
	assert.Contains(t, summary, "Recent commits:")
}
