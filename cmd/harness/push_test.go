package main

import (
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/evidence"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
