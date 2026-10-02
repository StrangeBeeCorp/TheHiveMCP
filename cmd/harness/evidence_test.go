package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

// The evidence names the commit it compared with, and the Gate evidence says whether the change
// is one a model reads: the first snapshot is not, an edit of an existing one is.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestTheEvidenceNamesItsBaseAndWhetherAModelReadsTheChange(t *testing.T) {
	dir := shipRepo(t)
	onMain := gittest.Git(t, dir, "rev-parse", "origin/main")
	commitFiles(t, dir, map[string]string{checks.SurfaceSummary: "{\"tools_total\": 0}\n"})
	head := reviewed(t, dir, "ready")

	_, err := harness(t, "pr", "--push=false")
	require.NoError(t, err)

	ev, err := evidence.Read(dir, head)
	require.NoError(t, err)
	require.NotNil(t, ev)
	assert.Equal(t, onMain, ev.Base, "the commit origin/main named when the gates ran")
	assert.True(t, ev.FirstSnapshot, "origin/main has no snapshot")

	out, err := harness(t, "evidence", "--markdown")
	require.NoError(t, err)
	assert.Contains(t, out, "compared with `"+onMain[:7]+"`")
	assert.Contains(t, out, "**Model-facing: no.** The base had no `baselines/surface/`")

	gittest.Git(t, dir, "checkout", "-q", "main")
	commitFiles(t, dir, map[string]string{checks.SurfaceSummary: "{\"tools_total\": 0}\n"})
	gittest.Git(t, dir, "push", "-q", "origin", "main")
	gittest.Git(t, dir, "checkout", "-q", "-B", "work", "origin/main")
	commitFiles(t, dir, map[string]string{checks.SurfaceSummary: "{\"tools_total\": 1}\n"})
	head = reviewed(t, dir, "ready")

	_, err = harness(t, "pr", "--push=false")
	require.NoError(t, err)

	ev, err = evidence.Read(dir, head)
	require.NoError(t, err)
	require.NotNil(t, ev)
	assert.False(t, ev.FirstSnapshot, "origin/main has a snapshot now")

	out, err = harness(t, "evidence", "--markdown")
	require.NoError(t, err)
	assert.Contains(t, out, "**Model-facing: yes.**", "the change edits what the model reads")
}

// A gate that changes the checkout it judges leaves no evidence: the record would name a commit, or
// a tree, the gates did not finish on.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestAGateThatChangesTheCheckoutLeavesNoEvidence(t *testing.T) {
	for name, runs := range map[string]string{
		"a dirty tree": "touch dirty.txt",
		"a new commit": "git -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m moved",
	} {
		t.Run(name, func(t *testing.T) {
			dir := shipRepo(t)
			commitFiles(t, dir, map[string]string{registryPath: strings.Replace(shipRegistry, `"test ! -e fail.flag"`, `"`+runs+`"`, 1)})
			head := reviewed(t, dir, "ready")

			_, err := harness(t, "pr")
			require.ErrorIs(t, err, errChangedDuringRun)

			ev, err := evidence.Read(dir, head)
			require.NoError(t, err)
			assert.Nil(t, ev, "nothing is recorded for a commit the gates did not finish on")
			assert.Empty(t, pushed(t, dir))
		})
	}
}

// The review skill asks the harness how hard to review: the highest effort of the suites the
// change selects.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestReviewEffortFollowsTheChangedPaths(t *testing.T) {
	shipRepo(t)

	out, err := harness(t, "review-effort")
	require.NoError(t, err)
	assert.Equal(t, "low\n", out, "the branch edits a doc")

	out, err = harness(t, "review-effort", "--all")
	require.NoError(t, err)
	assert.Equal(t, "high\n", out, "every suite, the harness's own among them")
}

// A branch rebased before its first push keeps the review the review state still holds for it.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestAReviewNotYetKeptCoversTheBranchRebased(t *testing.T) {
	dir := shipRepo(t)
	reviewed(t, dir, "ready")

	gittest.Git(t, dir, "checkout", "-q", "main")
	commitFiles(t, dir, map[string]string{"docs/b.md": "# below\n"})
	gittest.Git(t, dir, "push", "-q", "origin", "main")
	gittest.Git(t, dir, "checkout", "-q", "work")
	gittest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "rebase", "-q", "origin/main")

	kept, err := evidence.Kept(dir)
	require.NoError(t, err)
	require.Empty(t, kept, "nothing pushed yet, so nothing kept")

	rebased := gitx.Head(t.Context(), dir)

	out, err := harness(t, "pr")
	require.NoError(t, err, out)
	assert.Contains(t, out, "covers "+rebased[:7]+": the same change, rebased onto origin/main")
	assert.Contains(t, pushed(t, dir), rebased)
}

// make pr judges the whole change: neither $HARNESS_BASE, which CI sets, nor a flag narrows what its
// verdict covers.
//

func TestPrJudgesTheBranchAgainstOriginMainWhateverTheBase(t *testing.T) {
	dir := shipRepo(t)
	onMain := gittest.Git(t, dir, "rev-parse", "origin/main")
	gittest.Git(t, dir, "branch", "below", "HEAD")
	t.Setenv("HARNESS_BASE", "below")

	head := reviewed(t, dir, "ready")

	_, err := harness(t, "pr", "--push=false")
	require.NoError(t, err)

	ev, err := evidence.Read(dir, head)
	require.NoError(t, err)
	require.NotNil(t, ev)
	assert.Equal(t, onMain, ev.Base, "origin/main, not $HARNESS_BASE")
	assert.Contains(t, ev.Changed, docA, "against below, the branch would change nothing")

	_, err = harness(t, "pr", "--base", "below")
	require.ErrorAs(t, err, &usageError{}, "make pr takes no base")
}

// A person who passes --person ships without a ready review, and is told what is missing.
//
//nolint:paralleltest // t.Chdir: the harness finds its repository from the working directory
func TestPrForAPersonShipsWithAWarning(t *testing.T) {
	dir := shipRepo(t)
	head := reviewed(t, dir, "changes_needed")

	_, err := harness(t, "pr")
	require.ErrorIs(t, err, evidence.ErrNotShippable)
	assert.Empty(t, pushed(t, dir))

	out, err := harness(t, "pr", "--person")
	require.NoError(t, err, out)
	assert.Contains(t, out, "warning:")
	assert.Contains(t, pushed(t, dir), head)
}
