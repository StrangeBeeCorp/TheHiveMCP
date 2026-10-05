package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/bench"
)

// The base's benchmarks run through this checkout's make, from this checkout, so that its Makefile
// finds scripts/harness.mk (a base before the harness has none), with the base's scratch checkout
// mounted as the one to run in.
func TestTheBaseBenchmarksRunThroughThisCheckoutsMake(t *testing.T) {
	t.Parallel()

	b := bench.Budgets{Packages: []string{"./internal/utils"}, Count: 5, Benchtime: "300ms"}
	cmd := benchCommand(t.Context(), "/repo", "/repo/.cache/scratch/base", b)

	assert.Equal(t, "/repo", cmd.Dir, "make runs where the Makefile's includes are")
	assert.Equal(t, []string{
		"make", "--no-print-directory", "go",
		"CMD=test -run ^NoTest -bench . -benchmem -count 5 -benchtime 300ms ./internal/utils",
		"MOUNT_APP=/repo/.cache/scratch/base:/app",
	}, cmd.Args)
}
