package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/registry"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/runner"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	prGate   = "lint"
	shipHead = "1111111aaaaaaa"
	moved    = "2222222bbbbbbb"
)

// shipping is an env on an empty checkout with one pr gate, and the git directory that holds
// its review state.
func shipping(t *testing.T) (env, string) {
	t.Helper()

	reg := &registry.Registry{Gates: []registry.Gate{{ID: prGate, Cadence: []string{"pr"}}}}

	return env{root: t.TempDir(), reg: reg}, t.TempDir()
}

func TestAPushWithoutEvidenceIsRefusedUnlessAPersonSaysSo(t *testing.T) {
	t.Parallel()

	e, gitDir := shipping(t)

	var out strings.Builder

	err := e.shippable(&out, shipHead, gitDir, true)
	require.ErrorIs(t, err, evidence.ErrNotShippable)
	assert.Contains(t, err.Error(), "--person")

	require.NoError(t, e.shippable(&out, shipHead, gitDir, false), "a person gets warnings")
	assert.Contains(t, out.String(), "warning: the pr gates have not run")
	assert.Contains(t, out.String(), "warning: no adversarial review")
}

func TestAPushWithTheEvidenceAndAReadyReviewOfHEADGoesAhead(t *testing.T) {
	t.Parallel()

	e, gitDir := shipping(t)

	_, err := evidence.Evidence{
		Head: shipHead, Verdict: runner.Pass, Results: []runner.Result{{Gate: prGate, Verdict: runner.Pass}},
	}.Write(e.root)
	require.NoError(t, err)

	review := filepath.Join(gitDir, evidence.ReviewPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(review), 0o750))
	require.NoError(t, os.WriteFile(review, []byte(`{"verdict": "ready", "verdict_head": "`+shipHead+`"}`), 0o600))

	var out strings.Builder

	require.NoError(t, e.shippable(&out, shipHead, gitDir, true))
	assert.Empty(t, out.String())

	err = e.shippable(&out, moved, gitDir, true)
	require.ErrorIs(t, err, evidence.ErrNotShippable, "the evidence and the review are for another commit")
}

func TestTheEvidenceIsForTheCommitTheGatesRanOn(t *testing.T) {
	t.Parallel()

	require.NoError(t, unchanged(shipHead, shipHead, true))
	require.ErrorIs(t, unchanged(shipHead, moved, true), errChangedDuringRun, "HEAD moved")
	require.ErrorIs(t, unchanged(shipHead, shipHead, false), errChangedDuringRun, "a file changed")
}

func TestNothingPushesMainOrAReleaseBranch(t *testing.T) {
	t.Parallel()

	for _, branch := range []string{mainBranch, "release/1.2", ""} {
		_, err := pushTarget(branch)
		require.Error(t, err, "branch %q", branch)
	}

	target, err := pushTarget("feat/DL-8045-pr-flow")
	require.NoError(t, err)
	assert.Equal(t, "feat/DL-8045-pr-flow", target)
}

func TestTheGateEvidenceFlagsThePolicysProtectedClasses(t *testing.T) {
	t.Parallel()

	flags := evidence.Flags([]string{
		"spec/items.yaml", "gates/limits.yaml", "baselines/surface/tokens.json", "internal/harness/policy/paths.go",
		".github/CODEOWNERS", "internal/permissions/embedded/default_permissions.yaml", "internal/tools/search/handler.go",
	}, classOf)

	classes := make([]string, 0, len(flags))
	for _, f := range flags {
		classes = append(classes, f.Class)
	}

	assert.Equal(t, []string{"baselines", "gates", "harness policy", "spec", "trust"}, classes)
	assert.Len(t, flags[2].Paths, 2, "the harness and its workflows")
}
