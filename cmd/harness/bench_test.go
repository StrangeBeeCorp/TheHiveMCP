package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/bench"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

// utilsPkg is the package the benchmark commands of these tests run.
const utilsPkg = "./internal/utils"

// The command really runs: this checkout's make, dry-run, finds the go target in the
// scripts/harness.mk it includes, although the base's scratch checkout has neither, and the docker
// command it would run mounts that scratch checkout as /app.
func TestTheBaseBenchmarksMakeResolvesFromThisCheckout(t *testing.T) {
	t.Parallel()

	_, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make is not installed here (the Debian Go image and CI's runner have it)")
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	base := t.TempDir() // a checkout with no Makefile and no scripts/harness.mk, as origin/main before the harness
	cmd := benchCommand(t.Context(), root, base, bench.Budgets{Packages: []string{utilsPkg}, Count: 1, Benchtime: "1x"})
	cmd.Args = append(cmd.Args, "--dry-run")

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Contains(t, string(out), "-v "+base+":/app", "the scratch checkout is what runs")
	assert.Contains(t, string(out), "go test -run ^NoTest -bench . -benchmem -count 1 -benchtime 1x ./internal/utils")
}

// With no merge base, or a base where the benchmarks do not run, there is nothing to compare
// with: baseBench says so and returns no results, so that only the absolute budgets hold.
func TestWithoutABaseToRunOnlyTheAbsoluteBudgetsHold(t *testing.T) {
	t.Parallel()

	_, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make is not installed here (the Debian Go image and CI's runner have it)")
	}

	b := bench.Budgets{Packages: []string{utilsPkg}, Count: 1, Benchtime: "1x"}

	for name, c := range map[string]struct{ base, makefile, says string }{
		"no merge base":             {"nowhere", "go:\n\techo ran\n", "no merge base with nowhere: only the absolute budgets hold\n"},
		"the base does not run":     {onMain, "go:\n\t@echo no go here; exit 2\n", "the benchmarks do not run at the base: "},
		"the base has none of them": {onMain, "go:\n\t@echo PASS\n", " has none of these benchmarks: only the absolute budgets hold\n"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := gittest.Repo(t, map[string]string{makefile: c.makefile})

			var out bytes.Buffer

			assert.Empty(t, env{root: dir}.baseBench(t.Context(), c.base, b, &out))
			assert.Contains(t, out.String(), c.says)
		})
	}
}

// The base's benchmarks run through this checkout's make, from this checkout, so that its Makefile
// finds scripts/harness.mk (a base before the harness has none), with the base's scratch checkout
// mounted as the one to run in.
func TestTheBaseBenchmarksRunThroughThisCheckoutsMake(t *testing.T) {
	t.Parallel()

	b := bench.Budgets{Packages: []string{utilsPkg}, Count: 5, Benchtime: "300ms"}
	cmd := benchCommand(t.Context(), "/repo", "/repo/.cache/scratch/base", b)

	assert.Equal(t, "/repo", cmd.Dir, "make runs where the Makefile's includes are")
	assert.Equal(t, []string{
		"make", "--no-print-directory", "go",
		"CMD=test -run ^NoTest -bench . -benchmem -count 5 -benchtime 300ms ./internal/utils",
		"MOUNT_APP=/repo/.cache/scratch/base:/app",
	}, cmd.Args)
}
