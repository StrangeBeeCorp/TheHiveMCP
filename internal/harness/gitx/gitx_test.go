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
	docA   = "docs/a.md"
	goFile = "internal/x.go"
	newDoc = "new.md"
)

// base is a repository whose main branch has a doc, a Go file and an ignore rule, with a work
// branch checked out on top of it.
func base(t *testing.T) (string, string) {
	t.Helper()

	dir := gittest.Repo(t, map[string]string{
		docA:         "# a\n",
		goFile:       "package x\n",
		".gitignore": "*.log\n",
	})
	gittest.Git(t, dir, "checkout", "-q", "-b", "work")

	return dir, gitx.Head(t.Context(), dir)
}

// The gates are selected by what the branch changed since its base: committed, edited, staged
// or new, but not what git ignores.
func TestChangedSinceListsWhatTheBranchChangedCommittedOrNot(t *testing.T) {
	t.Parallel()

	dir, _ := base(t)

	gittest.Write(t, dir, map[string]string{docA: "# a, edited\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)

	changed, err := gitx.ChangedSince(t.Context(), dir, "main")
	require.NoError(t, err)
	assert.Equal(t, []string{docA}, changed, "a committed edit")

	gittest.Write(t, dir, map[string]string{goFile: "package x // edited\n", "internal/new.go": "package x\n", "run.log": "noise\n"})

	changed, err = gitx.ChangedSince(t.Context(), dir, "main")
	require.NoError(t, err)
	assert.Equal(t, []string{docA, "internal/new.go", goFile}, changed, "plus an uncommitted edit and a new file, not an ignored one")

	gittest.Git(t, dir, "rm", "-q", docA)

	changed, err = gitx.ChangedSince(t.Context(), dir, "main")
	require.NoError(t, err)
	assert.Contains(t, changed, docA, "a deletion is a change")
}

// The merge base is the comparison point, so work on main after the branch started is not the
// branch's change.
func TestChangedSinceComparesWithTheMergeBase(t *testing.T) {
	t.Parallel()

	dir, _ := base(t)

	gittest.Git(t, dir, "checkout", "-q", "main")
	gittest.Write(t, dir, map[string]string{"docs/main-only.md": "# later on main\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)
	gittest.Git(t, dir, "checkout", "-q", "work")

	changed, err := gitx.ChangedSince(t.Context(), dir, "main")
	require.NoError(t, err)
	assert.Empty(t, changed)
}

// The work not committed yet is the edited, the staged and the new files, not the ignored ones.
func TestUncommittedListsTheWorkNotCommittedYet(t *testing.T) {
	t.Parallel()

	dir, _ := base(t)
	gittest.Write(t, dir, map[string]string{docA: "# edited\n", "staged.md": "# s\n", newDoc: "# n\n", "run.log": "noise\n"})
	gittest.Git(t, dir, "add", "staged.md")

	got, err := gitx.Uncommitted(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, []string{docA, newDoc, "staged.md"}, got)
}

// A commit's files are its change against its first parent: a merge's are what it brought in,
// and the root commit's every file it added.
func TestCommittedListsACommitsFilesAgainstItsFirstParent(t *testing.T) {
	t.Parallel()

	dir, _ := base(t)

	root, err := gitx.Committed(t.Context(), dir, "HEAD")
	require.NoError(t, err)
	assert.Equal(t, []string{ignoreRules, docA, goFile}, root, "the root commit")

	gittest.Write(t, dir, map[string]string{"work.md": "# w\n"})
	gittest.Git(t, dir, "add", "work.md")
	gittest.Commit(t, dir)
	gittest.Git(t, dir, "checkout", "-q", "main")
	gittest.Write(t, dir, map[string]string{"main.md": "# m\n"})
	gittest.Git(t, dir, "add", "main.md")
	gittest.Commit(t, dir)
	gittest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "merge", "-q", "--no-ff", "--no-edit", "work")

	merged, err := gitx.Committed(t.Context(), dir, "HEAD")
	require.NoError(t, err)
	assert.Equal(t, []string{"work.md"}, merged, "what the merge brought into main")
}

// The pre-commit hook judges what the commit carries: the staged paths and the staged content,
// whatever the working tree holds.
func TestStagedAndIndexDescribeTheNextCommit(t *testing.T) {
	t.Parallel()

	dir, _ := base(t)

	gittest.Write(t, dir, map[string]string{docA: "# staged\n"})
	gittest.Git(t, dir, "add", docA)
	gittest.Write(t, dir, map[string]string{docA: "# not staged\n", goFile: "package y\n"})

	staged, err := gitx.Staged(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, []string{docA}, staged)

	index, err := gitx.Index(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, "# staged\n", string(index[docA]))
	assert.Equal(t, "package x\n", string(index[goFile]), "the index, not the working tree")
	assert.Len(t, index, 3)
}

func TestFilesListsTrackedAndNewFilesButNotIgnoredOnes(t *testing.T) {
	t.Parallel()

	dir, _ := base(t)
	gittest.Write(t, dir, map[string]string{newDoc: "x\n", "debug.log": "x\n"})

	files, err := gitx.Files(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, []string{".gitignore", docA, goFile, newDoc}, files)
}

func TestRootHeadAndHasRef(t *testing.T) {
	t.Parallel()

	dir, head := base(t)

	root, err := gitx.Root(t.Context(), filepath.Join(dir, "docs"))
	require.NoError(t, err)

	want, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	got, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	assert.Equal(t, want, got, "the root from a subdirectory")

	assert.Len(t, head, 40)
	assert.True(t, gitx.HasRef(t.Context(), dir, "main"))
	assert.False(t, gitx.HasRef(t.Context(), dir, "origin/main"), "no such ref")

	_, err = gitx.Root(t.Context(), t.TempDir())
	require.Error(t, err, "not a repository")

	fresh := t.TempDir()
	gittest.Git(t, fresh, "init", "-q")
	assert.Empty(t, gitx.Head(t.Context(), fresh), "before the first commit")
}

// A rename lists both paths, staged or committed, so the suite that owned the old one is selected.
func TestARenameListsBothPaths(t *testing.T) {
	t.Parallel()

	dir, _ := base(t)
	gittest.Git(t, dir, "mv", docA, "internal/a.md")

	staged, err := gitx.Staged(t.Context(), dir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{docA, "internal/a.md"}, staged, "staged")

	gittest.Commit(t, dir)

	changed, err := gitx.ChangedSince(t.Context(), dir, "main")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{docA, "internal/a.md"}, changed, "committed")
}

func TestIndexedReadsTheStagedContent(t *testing.T) {
	t.Parallel()

	dir, _ := base(t)
	gittest.Write(t, dir, map[string]string{docA: "# staged\n"})
	gittest.Git(t, dir, "add", docA)
	gittest.Write(t, dir, map[string]string{docA: "# on disk\n"})

	got, err := gitx.Indexed(t.Context(), dir, docA)
	require.NoError(t, err)
	assert.Equal(t, "# staged\n", string(got))

	_, err = gitx.Indexed(t.Context(), dir, "nowhere.md")
	require.Error(t, err)
}

// A rebase keeps a change's patch id; an edit to the change does not.
// origin/main is the remote's, written out, so that a tag or a branch of that name cannot stand in.
func TestRemoteNamesTheRemoteTrackingRef(t *testing.T) {
	t.Parallel()

	dir, head := base(t)
	assert.Equal(t, "origin/main", gitx.Remote(t.Context(), dir, "origin/main"), "no remote yet: the name as it is")

	gittest.Git(t, dir, "update-ref", "refs/remotes/origin/main", head)
	assert.Equal(t, "refs/remotes/origin/main", gitx.Remote(t.Context(), dir, "origin/main"))
	assert.Equal(t, "HEAD", gitx.Remote(t.Context(), dir, "HEAD"))
	assert.Equal(t, "refs/heads/main", gitx.Remote(t.Context(), dir, "refs/heads/main"))
}

// The evidence names the commit its base was, not the ref, which moves.
func TestResolveNamesTheCommitARefNames(t *testing.T) {
	t.Parallel()

	dir, head := base(t)
	assert.Equal(t, head, gitx.Resolve(t.Context(), dir, "main"))
	assert.Equal(t, head, gitx.Resolve(t.Context(), dir, "HEAD"))
	assert.Empty(t, gitx.Resolve(t.Context(), dir, "nowhere"), "no such ref")
	assert.Empty(t, gitx.Resolve(t.Context(), dir, ""), "no base")
	assert.Empty(t, gitx.Resolve(t.Context(), dir, "HEAD^{tree}"), "a tree is not a commit")
}

func TestPatchIDNamesAChangeWhateverItSitsOn(t *testing.T) {
	t.Parallel()

	dir, _ := base(t)
	gittest.Write(t, dir, map[string]string{docA: "# a, edited\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)

	start := gittest.Git(t, dir, "merge-base", "main", "HEAD")
	before := gitx.PatchID(t.Context(), dir, start+"..HEAD")
	require.NotEmpty(t, before)
	assert.Equal(t, before, gitx.PatchID(t.Context(), dir, "main...HEAD"), "from the merge base")

	gittest.Git(t, dir, "checkout", "-q", "main")
	gittest.Write(t, dir, map[string]string{"docs/b.md": "# b\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)
	gittest.Git(t, dir, "checkout", "-q", "work")
	gittest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "rebase", "-q", "main")
	assert.Equal(t, before, gitx.PatchID(t.Context(), dir, "main...HEAD"), "rebased onto a newer main")

	gittest.Write(t, dir, map[string]string{docA: "# a, edited twice\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)
	assert.NotEqual(t, before, gitx.PatchID(t.Context(), dir, "main...HEAD"), "edited")

	edited := gitx.PatchID(t.Context(), dir, "main...HEAD")
	gittest.Write(t, dir, map[string]string{docA: "#  a, edited twice\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)
	assert.NotEqual(t, edited, gitx.PatchID(t.Context(), dir, "main...HEAD"), "a space is an edit: an indent changes YAML")

	assert.Empty(t, gitx.PatchID(t.Context(), dir, "HEAD..HEAD"), "nothing changed")
	assert.Empty(t, gitx.PatchID(t.Context(), dir, "0123456789012345678901234567890123456789..HEAD"), "an unknown commit")
	assert.Empty(t, gitx.PatchID(t.Context(), dir, "--output=/tmp/x"), "an option is not a range")
}
