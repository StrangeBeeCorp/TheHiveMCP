package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shipRegistry has one pr gate, which fails while the repository has a fail.flag file.
const shipRegistry = `
scopes: {edit: a, stop: a, commit: a, pr: a, ci: a, nightly: a}
suites:
  - {id: everything, review: high, paths: [gates/**]}
  - {id: docs, review: low, paths: [docs/**, fail.flag]}
gates:
  - {id: docs-check, runs: "test ! -e fail.flag", cadence: [pr], suites: ["*"], budget_s: 10, blocks: x}
`

// shipRepo is a checkout whose origin is a bare repository holding main, on a work branch with one
// commit that edits a doc. It is the working directory.
func shipRepo(t *testing.T) string {
	t.Helper()

	origin := t.TempDir()
	gittest.Git(t, origin, "init", "-q", "--bare", "-b", "main")

	dir := gittest.Repo(t, map[string]string{
		registryPath: shipRegistry, limitsPath: "hooks: {max_red_rounds: 6}\n", ".gitignore": ".cache/\n", docA: "# a\n",
	})
	gittest.Git(t, dir, "remote", "add", "origin", origin)
	gittest.Git(t, dir, "push", "-q", "origin", "main")
	gittest.Git(t, dir, "fetch", "-q", "origin")
	gittest.Git(t, dir, "checkout", "-q", "-b", "work")
	commitFiles(t, dir, map[string]string{docA: editedA})

	t.Chdir(dir)
	t.Setenv("CI", "")
	t.Setenv("HARNESS_BASE", "")

	return dir
}

func commitFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	gittest.Write(t, dir, files)
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)
}

// reviewed writes the review state the review skill would leave: verdict for HEAD, compared with
// origin/main.
func reviewed(t *testing.T, dir, verdict string) string {
	t.Helper()

	head := gitx.Head(t.Context(), dir)
	state, err := json.Marshal(evidence.Review{
		Verdict: verdict, Head: head, Base: "origin/main", MergeBase: gittest.Git(t, dir, "merge-base", "origin/main", "HEAD"),
	})
	require.NoError(t, err)

	p := filepath.Join(dir, ".git", evidence.ReviewPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, state, 0o600))

	return head
}

func pushed(t *testing.T, dir string) string {
	t.Helper()
	return gittest.Git(t, dir, "ls-remote", "origin", "refs/heads/work")
}

func recorded(t *testing.T, dir, head string) runner.Verdict {
	t.Helper()

	ev, err := evidence.Read(dir, head)
	require.NoError(t, err)
	require.NotNil(t, ev, "make pr records the evidence, pass or fail")

	return ev.Verdict
}

//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestPrRunsTheGatesRecordsTheEvidenceAndPushes(t *testing.T) {
	dir := shipRepo(t)
	head := reviewed(t, dir, "ready")

	out, err := harness(t, "pr")
	require.NoError(t, err, out)

	assert.Equal(t, runner.Pass, recorded(t, dir, head))
	assert.Contains(t, pushed(t, dir), head, "the branch reached origin")

	kept, err := evidence.Kept(dir)
	require.NoError(t, err)
	require.Len(t, kept, 1, "the push keeps the review that let it ship")
	assert.Equal(t, head, kept[0].Head)
}

// make pr notes its gate times on the commit, under this machine's notes ref, and the push takes
// the ref to origin with the branch: another clone reads them there.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestPrNotesItsTimingsAndPushesThem(t *testing.T) {
	dir := shipRepo(t)
	head := reviewed(t, dir, "ready")

	out, err := harness(t, "pr")
	require.NoError(t, err, out)

	note := gitx.Note(t.Context(), dir, evidence.NotesRef(), head)
	require.NotEmpty(t, note, "the run is noted on its commit: %s", out)

	var n evidence.Note
	require.NoError(t, json.Unmarshal([]byte(note), &n))
	assert.Equal(t, head, n.Head)
	require.Len(t, n.Gates, 1)
	assert.Equal(t, "docs-check", n.Gates[0].Gate)
	assert.Positive(t, n.Machine.CPUs)

	remote := gittest.Git(t, dir, "ls-remote", "origin", evidence.NotesRef())
	assert.NotEmpty(t, remote, "the notes ref reached origin")

	clone := filepath.Join(t.TempDir(), "clone")
	gittest.Git(t, dir, "clone", "-q", gittest.Git(t, dir, "remote", "get-url", "origin"), clone)
	gittest.Git(t, clone, "fetch", "-q", "origin", evidence.NotesRef()+":"+evidence.NotesRef())
	assert.Equal(t, note, gitx.Note(t.Context(), clone, evidence.NotesRef(), head), "another clone reads the same timings")
}

//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestPrPushesNothingWhenAGateFailsOrTheReviewIsNotReady(t *testing.T) {
	dir := shipRepo(t)
	commitFiles(t, dir, map[string]string{"fail.flag": "x\n"})
	head := reviewed(t, dir, "ready")

	_, err := harness(t, "pr")
	require.ErrorIs(t, err, errFailed)
	assert.Equal(t, runner.Fail, recorded(t, dir, head))
	assert.Empty(t, pushed(t, dir))

	gittest.Git(t, dir, "rm", "-q", "fail.flag")
	gittest.Commit(t, dir)

	head = reviewed(t, dir, "changes_needed")

	_, err = harness(t, "pr")
	require.ErrorIs(t, err, evidence.ErrNotShippable)
	assert.Equal(t, runner.Pass, recorded(t, dir, head), "the gates passed")
	assert.Empty(t, pushed(t, dir), "but the review did not")
}

//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestPrRefusesAnUncommittedTreeAndMain(t *testing.T) {
	dir := shipRepo(t)
	reviewed(t, dir, "ready")
	gittest.Write(t, dir, map[string]string{"notes.md": "draft\n"})

	_, err := harness(t, "pr")
	require.ErrorAs(t, err, &usageError{})
	assert.Contains(t, err.Error(), "commit first")

	require.NoError(t, os.Remove(filepath.Join(dir, "notes.md")))
	gittest.Git(t, dir, "checkout", "-q", "main")

	_, err = harness(t, "pr")
	require.ErrorAs(t, err, &usageError{})
	assert.Contains(t, err.Error(), "changes only when a person merges")
}

// A stacked pull request is rebased when the base moves. The review state has moved on to another
// branch by then: the review the first push kept still covers the branch's commits, replayed as it
// read them, replayed with a resolved conflict, or added since. A rebase asks for no new review;
// the push and the evidence list what the review did not read, for whoever merges.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestTheReviewOfABranchCoversItRebasedAndListsWhatItDidNotRead(t *testing.T) {
	dir := shipRepo(t)
	reviewed(t, dir, "ready")

	out, err := harness(t, "pr")
	require.NoError(t, err, out)

	// main changes the line the branch edits: the rebase stops on a conflict, resolved by hand.
	gittest.Git(t, dir, "checkout", "-q", "main")
	commitFiles(t, dir, map[string]string{docA: "# a, upstream\n", "docs/b.md": "# below\n"})
	gittest.Git(t, dir, "push", "-q", "origin", "main")
	gittest.Git(t, dir, "checkout", "-q", "work")
	conflict := exec.CommandContext(t.Context(), "git", "-c", "user.name=t", "-c", "user.email=t@example.com", "rebase", "-q", "origin/main")
	conflict.Dir = dir
	require.Error(t, conflict.Run(), "the rebase conflicts")
	gittest.Write(t, dir, map[string]string{docA: "# a, upstream and edited\n"})
	gittest.Git(t, dir, "add", docA)
	gittest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "core.editor=true", "rebase", "--continue")
	commitFiles(t, dir, map[string]string{"docs/c.md": "# added after the review\n"})
	require.NoError(t, os.Remove(filepath.Join(dir, ".git", evidence.ReviewPath)), "the review state moved on")

	head := gitx.Head(t.Context(), dir)

	out, err = harness(t, "pr", "--force")
	require.NoError(t, err, out)
	assert.Contains(t, out, "covers "+head[:7]+", which it did not judge: 0 commit(s) as it read them, 1 changed by the rebase, 1 added since and not reviewed")
	assert.Contains(t, pushed(t, dir), head, "no new review was needed")

	ev, err := evidence.Read(dir, head)
	require.NoError(t, err)
	require.NotNil(t, ev.Coverage, "the evidence records what the review did not read")
	assert.Len(t, ev.Coverage.Rebased, 1)
	require.Len(t, ev.Coverage.Added, 1)
	assert.Equal(t, head, ev.Coverage.Added[0].SHA)
}

// A branch none of whose commits a ready review read is refused, whatever reviews the checkout
// kept for other branches.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestABranchNoReviewReadIsRefused(t *testing.T) {
	dir := shipRepo(t)
	reviewed(t, dir, "ready")

	out, err := harness(t, "pr")
	require.NoError(t, err, out)

	gittest.Git(t, dir, "checkout", "-q", "-b", "other", "main")
	gittest.Write(t, dir, map[string]string{"docs/other.md": "# another change\n"})
	gittest.Git(t, dir, "add", "-A")
	gittest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false",
		"-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "docs: another branch's change")
	require.NoError(t, os.Remove(filepath.Join(dir, ".git", evidence.ReviewPath)))

	_, err = harness(t, "pr")
	require.ErrorIs(t, err, evidence.ErrNotShippable, "the kept review read work's commits, not other's")
	assert.Empty(t, gittest.Git(t, dir, "ls-remote", "origin", "refs/heads/other"))
}

//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestTheGateEvidenceFlagsWhatThePullRequestChangesAgainstItsBase(t *testing.T) {
	dir := shipRepo(t)

	gittest.Git(t, dir, "checkout", "-q", "-b", "below", "main")
	commitFiles(t, dir, map[string]string{"gates/extra.yaml": "x: 1\n"})
	gittest.Git(t, dir, "checkout", "-q", "work")
	gittest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "rebase", "-q", "below")
	reviewed(t, dir, "ready")

	_, err := harness(t, "pr", "--push=false")
	require.NoError(t, err)

	out, err := harness(t, "evidence", "--markdown")
	require.NoError(t, err)
	assert.Contains(t, out, "`gates/extra.yaml`", "against origin/main, the branch below is in the change")

	out, err = harness(t, "evidence", "--markdown", "--base", "below")
	require.NoError(t, err)
	assert.Contains(t, out, "the files and the model-facing note below are this pull request's own change, against `below`")
	assert.NotContains(t, out, "gates/extra.yaml", "the pull request stacked on below does not change it")

	_, err = harness(t, "evidence", "--markdown", "--base", "nowhere")
	require.ErrorAs(t, err, &usageError{})
}
