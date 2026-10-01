package evidence_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"
)

const (
	head  = "1111111aaaaaaa"
	other = "2222222bbbbbbb"
	ready = "ready"
)

func TestEvidenceRoundTripsByCommit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	ev := evidence.Evidence{Head: head, Verdict: runner.Pass, Results: []runner.Result{{Gate: "lint", Verdict: runner.Pass}}}

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

func TestAnAgentPushesOnlyWhatTheGatesAndTheReviewPassed(t *testing.T) {
	t.Parallel()

	passed := &evidence.Evidence{Head: head, Verdict: runner.Pass}
	reviewed := &evidence.Review{Verdict: ready, Head: head}

	_, err := evidence.PushAllowed(head, passed, reviewed, true)
	require.NoError(t, err)

	for name, c := range map[string]struct {
		ev   *evidence.Evidence
		rv   *evidence.Review
		want string
	}{
		"no evidence":           {nil, reviewed, "have not run at 1111111"},
		"evidence of another":   {&evidence.Evidence{Head: other, Verdict: runner.Pass}, reviewed, "have not run at 1111111"},
		"gates failed":          {&evidence.Evidence{Head: head, Verdict: runner.Fail}, reviewed, "were FAIL"},
		"gates incomplete":      {&evidence.Evidence{Head: head, Verdict: runner.Incomplete}, reviewed, "were INCOMPLETE"},
		"no review":             {passed, nil, "no adversarial review"},
		"review of another":     {passed, &evidence.Review{Verdict: ready, Head: other}, "judged 2222222 but HEAD is 1111111"},
		"review not finished":   {passed, &evidence.Review{Verdict: "continue", Head: head}, "verdict is continue"},
		"review needs a person": {passed, &evidence.Review{Verdict: "needs_you", Head: head}, "verdict is needs_you"},
	} {
		_, err := evidence.PushAllowed(head, c.ev, c.rv, true)
		require.ErrorIs(t, err, evidence.ErrNotShippable, name)
		assert.Contains(t, err.Error(), c.want, name)
	}
}

func TestAPersonIsWarnedNotStopped(t *testing.T) {
	t.Parallel()

	warnings, err := evidence.PushAllowed(head, nil, nil, false)
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
			{Gate: "lint", Verdict: runner.Pass, Seconds: 12.3, Summary: "lint | passed\nmore"},
			{Gate: "integration", Verdict: runner.Skipped, Summary: "not selected"},
		},
	}

	md := evidence.Markdown(ev, evidence.Flags(ev.Changed, classOf))

	assert.Contains(t, md, "`make pr` at `1111111` (`feat/x`, compared with `2222222`): **PASS**, 1 gates in 1m15s.")
	assert.Contains(t, md, "| lint | ✅ PASS | 12.3 s | lint \\| passed |")
	assert.Contains(t, md, "> - gates: `gates/limits.yaml`\n> - spec: `spec/items.yaml`")
	assert.Contains(t, md, "**Model-facing: yes.**")

	ev.Changed = []string{"internal/tools/search/handler.go"}
	md = evidence.Markdown(ev, evidence.Flags(ev.Changed, classOf))
	assert.NotContains(t, md, "CAUTION")
	assert.Contains(t, md, "**Model-facing: no.**")
}
