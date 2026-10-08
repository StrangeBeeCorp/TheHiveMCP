package gitx_test

import (
	"path/filepath"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const notesRef = "refs/notes/harness/test"

// A note is added under its ref, replaces the one the commit had there, and reads back; a commit
// without one reads "".
func TestANoteIsAddedReplacedAndRead(t *testing.T) {
	t.Parallel()

	dir := gittest.Repo(t, nil)
	head := gitx.Head(t.Context(), dir)

	assert.Empty(t, gitx.Note(t.Context(), dir, notesRef, head))

	require.NoError(t, gitx.AddNote(t.Context(), dir, notesRef, head, []byte("first\n")))
	require.NoError(t, gitx.AddNote(t.Context(), dir, notesRef, head, []byte("second\n")))
	assert.Equal(t, "second", gitx.Note(t.Context(), dir, notesRef, head))
	assert.Empty(t, gitx.Note(t.Context(), dir, "refs/notes/other", head), "another ref holds nothing")

	require.Error(t, gitx.AddNote(t.Context(), dir, notesRef, "no-such-revision", []byte("x")), "nothing to note")
}

// The checkout's config keeps what SetConfig sets, and an unset key reads "".
func TestConfigValueReadsWhatSetConfigKept(t *testing.T) {
	t.Parallel()

	dir := gittest.Repo(t, nil)

	assert.Empty(t, gitx.ConfigValue(t.Context(), dir, "harness.unset"))
	require.NoError(t, gitx.SetConfig(t.Context(), dir, "harness.kept", "abc"))
	assert.Equal(t, "abc", gitx.ConfigValue(t.Context(), dir, "harness.kept"))
	require.Error(t, gitx.SetConfig(t.Context(), dir, "not a key", "x"))
}

// Two clones that note the same commit under the same ref both push: the second merges what the
// remote holds, and the remote keeps both notes. A remote that cannot be reached is an error.
func TestPushNotesMergesWhatTheRemoteHolds(t *testing.T) {
	t.Parallel()

	first := gittest.Repo(t, nil)
	head := gitx.Head(t.Context(), first)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gittest.Git(t, first, "clone", "-q", "--bare", first, remote)
	gittest.Git(t, first, "remote", "add", "origin", remote)

	second := filepath.Join(t.TempDir(), "second")
	gittest.Git(t, first, "clone", "-q", remote, second)

	require.NoError(t, gitx.AddNote(t.Context(), first, notesRef, head, []byte("from the first\n")))
	require.NoError(t, gitx.PushNotes(t.Context(), first, "origin", notesRef), "the remote has no notes yet")

	require.NoError(t, gitx.AddNote(t.Context(), second, notesRef, head, []byte("from the second\n")))
	require.NoError(t, gitx.PushNotes(t.Context(), second, "origin", notesRef), "merged, not refused")

	check := filepath.Join(t.TempDir(), "check")
	gittest.Git(t, first, "clone", "-q", remote, check)
	gittest.Git(t, check, "fetch", "-q", "origin", notesRef+":"+notesRef)

	note := gitx.Note(t.Context(), check, notesRef, head)
	assert.Contains(t, note, "from the first")
	assert.Contains(t, note, "from the second")

	require.Error(t, gitx.PushNotes(t.Context(), first, filepath.Join(t.TempDir(), "absent"), notesRef))
}

// The diff is the branch's own change since its merge base, with no context; with no shared
// history there is no change to show.
func TestDiffIsTheBranchSinceItsMergeBase(t *testing.T) {
	t.Parallel()

	dir := gittest.Repo(t, map[string]string{"a.txt": "first\n"})
	gittest.Git(t, dir, "checkout", "-q", "-b", "work")
	gittest.Write(t, dir, map[string]string{"a.txt": "first\ntwo\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)

	patch, err := gitx.Diff(t.Context(), dir, "main")
	require.NoError(t, err)
	assert.Contains(t, patch, "+++ b/a.txt\n@@ -1,0 +2 @@ first\n+two\n")

	_, err = gitx.Diff(t.Context(), dir, "no-such-ref")
	require.ErrorIs(t, err, gitx.ErrNoMergeBase)
}
