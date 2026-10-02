package gitx_test

import (
	"path/filepath"
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

	gittest.Write(t, dir, map[string]string{"new.go": pkgSource})

	untracked := fingerprint()
	assert.NotEqual(t, edited, untracked)

	gittest.Write(t, dir, map[string]string{"new.go": "package x // other\n"})
	assert.NotEqual(t, untracked, fingerprint(), "an untracked file's content counts")

	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)

	committed := fingerprint()

	gittest.Commit(t, dir)
	assert.NotEqual(t, committed, fingerprint(), "a new HEAD")
}

// Branch is empty on a detached HEAD, MainCheckout names the main working tree from a linked
// one, Ignored follows .gitignore, and Summary lists the changes and the last commits.
func TestTheCheckoutIsDescribed(t *testing.T) {
	t.Parallel()

	dir := gittest.Repo(t, map[string]string{goFile: pkgSource, ignoreRules: ".cache/\n"})
	assert.Equal(t, "main", gitx.Branch(t.Context(), dir))
	assert.Equal(t, dir, filepath.Clean(gitx.MainCheckout(t.Context(), dir)))

	linked := filepath.Join(t.TempDir(), "linked")
	gittest.Git(t, dir, "worktree", "add", "-q", "-b", "feat/x", linked)
	assert.Equal(t, "feat/x", gitx.Branch(t.Context(), linked))
	assert.Equal(t, filepath.Clean(dir), filepath.Clean(gitx.MainCheckout(t.Context(), linked)))

	gittest.Git(t, dir, "checkout", "-q", "--detach")
	assert.Empty(t, gitx.Branch(t.Context(), dir))

	assert.True(t, gitx.Ignored(t.Context(), dir, ".cache/harness/BLOCKED.md"))
	assert.False(t, gitx.Ignored(t.Context(), dir, goFile))

	gittest.Write(t, dir, map[string]string{goFile: pkgEdited})
	summary := gitx.Summary(t.Context(), dir)
	assert.Contains(t, summary, "M "+goFile)
	assert.Contains(t, summary, "Recent commits:")
}
