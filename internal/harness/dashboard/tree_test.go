package dashboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

// A test is an integration test when its calls reach StartTheHiveContainer: through testutils'
// helpers, through its own package's, or inside a subtest. The others are unit tests, and fuzz
// and benchmark functions are counted by name.
func TestTestsAreSortedIntoLayersByWhatTheyReach(t *testing.T) {
	t.Parallel()

	tree := checks.MemTree{
		"internal/testutils/container.go": []byte("package testutils\nfunc StartTheHiveContainer() {}\n" +
			"func SetupTestWithCleanup() { StartTheHiveContainer() }\nfunc MockInput() {}\n"),
		"internal/tools/x_test.go": []byte("package tools\n" +
			"func setup() { testutils.SetupTestWithCleanup() }\n" +
			"func TestDirect(t *testing.T) { testutils.SetupTestWithCleanup() }\n" +
			"func TestThroughALocalHelper(t *testing.T) { setup() }\n" +
			"func TestInASubtest(t *testing.T) { t.Run(\"x\", func(t *testing.T) { setup() }) }\n" +
			"func TestUnit(t *testing.T) { testutils.MockInput() }\n" +
			"func TestMain(m *testing.M) {}\n" +
			"func FuzzX(f *testing.F) {}\nfunc BenchmarkX(b *testing.B) {}\n"),
		"internal/other/y_test.go": []byte("package other\nfunc setup() {}\nfunc TestSameHelperName(t *testing.T) { setup() }\n"),
		"broken_test.go":           []byte("package broken\nfunc TestX( {"),
	}

	l, err := countLayers(tree)
	require.NoError(t, err)
	assert.Equal(t, Layers{Unit: 2, Integration: 3, Fuzz: 1, Benchmark: 1}, l,
		"another package's setup, which reaches nothing, is a unit test's")
}

// The tree's numbers need the repository's limits: without them the source is not read.
func TestTheTreeNeedsTheRepositorysLimits(t *testing.T) {
	t.Parallel()

	_, err := readTree(t.Context(), t.TempDir())
	require.Error(t, err)
}

// The last make pr of this clone gives the coverage total and the mutation scores, whatever the
// length of its logs' lines; a clone that never ran make pr says so.
func TestTheLastLocalRunGivesCoverageAndMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	_, err := readLocal(root)
	require.ErrorIs(t, err, errNoRun)

	for name, files := range map[string]map[string]string{
		"pr-20261001-100000": {"results.json": `{"head":"old","verdict":"FAIL","started":"2026-10-01T10:00:00Z"}`},
		"pr-20261002-100000": {
			"results.json": `{"head":"new","verdict":"PASS","started":"2026-10-02T10:00:00Z"}`,
			"coverage.log": "=== RUN x\n" + string(make([]byte, 2<<20)) + "\ntotal:\t\t\t(statements)\t\t64.8%\n",
			"mutation.log": "internal/utils: efficacy 84.0% (floor 81%), 84 killed\nnoise\n",
		},
	} {
		dir := filepath.Join(root, ".cache", "harness", "runs", name)
		require.NoError(t, os.MkdirAll(dir, 0o750))

		for f, content := range files {
			require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte(content), 0o600))
		}
	}

	r, err := readLocal(root)
	require.NoError(t, err)
	assert.Equal(t, "new", r.Head, "the latest run")
	assert.InDelta(t, 64.8, r.Coverage, 0.01, "found after a 2 MiB line")
	assert.Equal(t, []Mutation{{Package: "internal/utils", Efficacy: 84, Floor: 81}}, r.Mutation)
}

// A tracked file deleted from the working tree is skipped, as the structure gate skips it; the
// other tests are still counted.
func TestADeletedFileLeavesTheOtherTestsCounted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package a\nfunc TestA(t *testing.T) {}\n"), 0o600))

	l, err := countLayers(checks.DirTree{Root: dir, Names: []string{"a_test.go", "gone_test.go"}})
	require.NoError(t, err)
	assert.Equal(t, Layers{Unit: 1}, l)
}

// A canary manifest that cannot be read is said; the tree's other numbers are kept.
func TestAnUnreadableCanaryManifestIsSaid(t *testing.T) {
	t.Parallel()

	limits, err := os.ReadFile(filepath.Join("..", "..", "..", "gates", "limits.yaml"))
	require.NoError(t, err)

	dir := gittest.Repo(t, map[string]string{"gates/limits.yaml": string(limits), "a_test.go": "package a\nfunc TestA(t *testing.T) {}\n"})

	d, err := readTree(t.Context(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the canaries")
	assert.Equal(t, 1, d.Layers.Unit, "the tests are still counted")
}
