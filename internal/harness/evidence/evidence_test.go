package evidence_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	head     = "1111111aaaaaaa"
	other    = "2222222bbbbbbb"
	ready    = "ready"
	lintGate = "lint"
	itGate   = "integration"
)

func TestEvidenceRoundTripsByCommit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	ev := evidence.Evidence{Head: head, Verdict: runner.Pass, Results: []runner.Result{{Gate: lintGate, Verdict: runner.Pass}}}

	_, err := ev.Write(root)
	require.NoError(t, err)

	got, err := evidence.Read(root, head)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, runner.Pass, got.Verdict)

	none, err := evidence.Read(root, other)
	require.NoError(t, err)
	assert.Nil(t, none, "a commit make pr never judged has no evidence")
}

var prGates = []string{lintGate, itGate}

func TestAnAgentPushesOnlyWhatTheGatesAndTheReviewPassed(t *testing.T) {
	t.Parallel()

	passed := &evidence.Evidence{Head: head, Verdict: runner.Pass, Results: []runner.Result{
		{Gate: lintGate, Verdict: runner.Pass}, {Gate: itGate, Verdict: runner.Skipped},
	}}
	reviewed := &evidence.Review{Verdict: ready, Head: head}

	_, err := evidence.PushAllowed(head, passed, reviewed, prGates, true)
	require.NoError(t, err)

	_, err = evidence.PushAllowed(head, passed, &evidence.Review{Verdict: "ready_with_known_issues", Head: head}, prGates, true)
	require.NoError(t, err, "known issues are shipped and stated, not blocked")

	for name, c := range map[string]struct {
		ev   *evidence.Evidence
		rv   *evidence.Review
		want string
	}{
		"no evidence":            {nil, reviewed, "have not run at 1111111"},
		"evidence of another":    {&evidence.Evidence{Head: other, Verdict: runner.Pass}, reviewed, "have not run at 1111111"},
		"gates failed":           {&evidence.Evidence{Head: head, Verdict: runner.Fail}, reviewed, "were FAIL"},
		"gates incomplete":       {&evidence.Evidence{Head: head, Verdict: runner.Incomplete}, reviewed, "were INCOMPLETE"},
		"no review":              {passed, nil, "no adversarial review"},
		"review of another":      {passed, &evidence.Review{Verdict: ready, Head: other}, "judged 2222222 but HEAD is 1111111"},
		"review not finished":    {passed, &evidence.Review{Verdict: "continue", Head: head}, "verdict is continue"},
		"review needs a person":  {passed, &evidence.Review{Verdict: "needs_you", Head: head}, "verdict is needs_you"},
		"review without verdict": {passed, &evidence.Review{Head: head}, "no adversarial review"},
		"review verdict unknown": {passed, &evidence.Review{Verdict: "readyish", Head: head}, "verdict is readyish"},
		"a gate without result": {
			&evidence.Evidence{Head: head, Verdict: runner.Pass, Results: []runner.Result{{Gate: lintGate, Verdict: runner.Pass}}},
			reviewed, "no result for [integration]",
		},
		"a hand-written record": {&evidence.Evidence{Head: head, Verdict: runner.Pass}, reviewed, "no result for [lint integration]"},
	} {
		_, err := evidence.PushAllowed(head, c.ev, c.rv, prGates, true)
		require.ErrorIs(t, err, evidence.ErrNotShippable, name)
		assert.Contains(t, err.Error(), c.want, name)
	}
}

func TestAPersonIsWarnedNotStopped(t *testing.T) {
	t.Parallel()

	warnings, err := evidence.PushAllowed(head, nil, nil, prGates, false)
	require.NoError(t, err)
	assert.Len(t, warnings, 2)
}

func TestTheEvidenceFlagsWhatOnlyAPersonChanges(t *testing.T) {
	t.Parallel()

	classOf := func(p string) (string, bool) {
		switch {
		case strings.HasPrefix(p, "gates/"):
			return "gates", true
		case strings.HasPrefix(p, "spec/"):
			return "spec", true
		}

		return "", false
	}

	ev := evidence.Evidence{
		Head: head, Base: other, Branch: "feat/x", Verdict: runner.Pass,
		Started: time.Unix(0, 0), Finished: time.Unix(75, 0),
		Changed: []string{"spec/items.yaml", "internal/tools/search/models.go", "gates/limits.yaml", "baselines/surface/tokens.json"},
		Results: []runner.Result{
			{Gate: lintGate, Verdict: runner.Pass, Seconds: 12.3, Summary: "\x1b[32mlint\x1b[0m | passed\nmore"},
			{Gate: itGate, Verdict: runner.Skipped, Summary: "not selected"},
		},
	}

	md := evidence.Markdown(ev, evidence.Flags(ev.Changed, classOf))

	assert.Contains(t, md, "`make pr` at `1111111` (`feat/x`, compared with `2222222`): **PASS**, 1 gates in 1m15s.")
	assert.Contains(t, md, "| lint | ✅ PASS | 12.3 s | lint \\| passed |")
	assert.Contains(t, md, "> - gates: `gates/limits.yaml`\n> - spec: `spec/items.yaml`")
	assert.Contains(t, md, "**Model-facing: yes.**")

	ev.FirstSnapshot = true
	assert.False(t, ev.ModelFacing(), "the first snapshot changes nothing a model reads")
	assert.Contains(t, evidence.Markdown(ev, nil), "**Model-facing: no.** The base had no `baselines/surface/`")

	ev.FirstSnapshot, ev.Changed = false, []string{"internal/tools/search/handler.go"}
	md = evidence.Markdown(ev, evidence.Flags(ev.Changed, classOf))
	assert.NotContains(t, md, "CAUTION")
	assert.Contains(t, md, "**Model-facing: no.**")

	ev.Base, ev.Changed = "", nil
	assert.Contains(t, evidence.Markdown(ev, nil), "**Model-facing: unknown.**", "no base: the harness does not know what changed")
}

func TestTheReviewStateIsReadAsTheReviewSkillWritesIt(t *testing.T) {
	t.Parallel()

	gitDir := t.TempDir()

	none, err := evidence.ReadReview(gitDir)
	require.NoError(t, err)
	assert.Nil(t, none)

	p := filepath.Join(gitDir, evidence.ReviewPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte(`{"effort": "high", "verdict": "ready", "verdict_head": "`+head+`", "heads": [],
		"base": "origin/main", "merge_base": "`+other+`"}`), 0o600))

	review, err := evidence.ReadReview(gitDir)
	require.NoError(t, err)
	assert.Equal(t, &evidence.Review{Verdict: ready, Head: head, Base: "origin/main", MergeBase: other}, review)

	require.NoError(t, os.WriteFile(p, []byte("{"), 0o600))

	_, err = evidence.ReadReview(gitDir)
	assert.Error(t, err)
}

func TestOnlyAReadyReviewLetsAChangeShip(t *testing.T) {
	t.Parallel()

	for verdict, want := range map[string]bool{
		ready: true, "ready_with_known_issues": true, "changes_needed": false, "needs_you": false, "continue": false, "": false,
	} {
		assert.Equal(t, want, evidence.Review{Verdict: verdict}.Ready(), verdict)
	}
}

// A push keeps the review that let it ship, so that the same change rebased still has it once
// the review state has moved on.
func TestKeptReviewsOutliveTheReviewState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	none, err := evidence.Kept(root)
	require.NoError(t, err)
	assert.Empty(t, none)

	first := evidence.Review{Verdict: ready, Head: head, Base: "origin/main", MergeBase: other}
	second := evidence.Review{Verdict: ready, Head: other, Base: "origin/feat/below", MergeBase: head}

	require.NoError(t, evidence.Keep(root, first))
	require.NoError(t, evidence.Keep(root, second))

	_, err = evidence.Evidence{Head: head, Verdict: runner.Pass}.Write(root)
	require.NoError(t, err)

	kept, err := evidence.Kept(root)
	require.NoError(t, err)
	assert.Equal(t, []evidence.Review{first, second}, kept, "the evidence next to them is not a review")

	require.NoError(t, os.WriteFile(filepath.Join(root, evidence.Dir, "review-broken.json"), []byte("{"), 0o600))

	_, err = evidence.Kept(root)
	assert.Error(t, err)
}

// A stacked pull request's evidence says which base selected the gates and which one its files
// are compared with.
func TestTheEvidenceOfAStackedPullRequestNamesBothBases(t *testing.T) {
	t.Parallel()

	ev := evidence.Evidence{Head: head, Base: other, Branch: "feat/top", Verdict: runner.Pass}
	assert.NotContains(t, evidence.Markdown(ev, nil), "selected against")

	ev.Against = "origin/feat/below"
	assert.Contains(t, evidence.Markdown(ev, nil), "The gates were selected against `2222222`; the files and the model-facing "+
		"note below are this pull request's own change, against `origin/feat/below`.")
}
