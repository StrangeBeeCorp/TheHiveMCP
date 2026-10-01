package main

import (
	"testing"

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
