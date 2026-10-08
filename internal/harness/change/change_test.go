package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/checks"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/gitx/gittest"
)

const aGo = "package m\n\n// A is one.\nfunc A() int {\n\treturn 1\n}\n"

// branch is a repository whose work branch, from main, lengthens a.go, adds a dependency, a
// //nolint and a TODO: its root.
func branch(t *testing.T) string {
	t.Helper()

	dir := gittest.Repo(t, map[string]string{"go.mod": "module example.com/m\n", aGoFile: aGo})
	gittest.Git(t, dir, "checkout", "-q", "-b", "work")
	gittest.Write(t, dir, map[string]string{
		"go.mod": "module example.com/m\n\nrequire example.com/dep v1.0.0\n",
		aGoFile:  aGo + "\n// B is two. TODO: say why.\nfunc B() int {\n\treturn 2 //nolint:mnd // a test\n}\n",
	})
	gittest.Git(t, dir, "add", "-A")
	gittest.Commit(t, dir)

	return dir
}

var lim = checks.StructureLimits{MaxLines: 12, CodeFiles: []string{"*.go"}}

// The metrics measure the branch since its merge base: the statements its changed lines fall in
// that the tests ran, the file it brings near its limit, and what it adds.
func TestMeasureMeasuresTheBranchSinceItsMergeBase(t *testing.T) {
	t.Parallel()

	dir := branch(t)
	profile := "mode: set\nexample.com/m/a.go:4.14,6.2 1 1\nexample.com/m/a.go:9.14,11.2 1 0\n"

	m, err := Measure(t.Context(), dir, "main", []byte(profile), lim)
	require.NoError(t, err)

	assert.Equal(t, gittest.Git(t, dir, "rev-parse", "main"), m.From)
	assert.Equal(t, gitx.Head(t.Context(), dir), m.Head)
	assert.Equal(t, &Coverage{Statements: 1}, m.Coverage, "only B changed, and the tests did not run it")
	assert.Equal(t, []Move{{Path: aGoFile, Before: 6, After: 11, Limit: 12}}, m.Moves)
	assert.Equal(t, Markers{Nolint: 1, TODO: 1}, m.Markers)
	assert.Equal(t, []string{"example.com/dep v1.0.0"}, m.Deps)

	m, err = Measure(t.Context(), dir, "main", nil, lim)
	require.NoError(t, err)
	assert.Nil(t, m.Coverage, "no profile: no diff coverage")
}

func TestMeasureFailsWithNoBaseOrABadProfile(t *testing.T) {
	t.Parallel()

	dir := branch(t)

	_, err := Measure(t.Context(), dir, "no-such-ref", nil, lim)
	require.ErrorIs(t, err, gitx.ErrNoMergeBase)

	_, err = Measure(t.Context(), dir, "main", []byte("mode: set\nnot a block\n"), lim)
	require.ErrorIs(t, err, ErrProfile)
}

func TestModuleIsWhatGoModDeclares(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "example.com/m", module([]byte("// c\nmodule \"example.com/m\"\n")))
	assert.Empty(t, module(nil))
}

// The section names the change it measured, each metric, and the files near their limit; what it
// could not measure it says.
func TestMarkdownSaysWhatTheChangeDoes(t *testing.T) {
	t.Parallel()

	m := Metrics{
		Against: "origin/main", From: "1111111aaaa", Head: "2222222bbbb",
		Coverage: &Coverage{Statements: 8, Covered: 6, Unmeasured: []string{"c.go"}},
		Moves:    []Move{{Path: aGoFile, Before: 400, After: 480, Limit: 500}, {Path: bGoFile, After: 400, Limit: 500}},
		Markers:  Markers{Nolint: 1, TODO: 2}, Deps: []string{"example.com/dep v1.0.0"},
	}

	out := Markdown(m)
	for _, want := range []string{
		"### Change metrics\n\nThis pull request's own change, from `1111111` (its merge base with `origin/main`) to `2222222`.",
		"| Diff coverage | **75.0 %**: the unit tests ran 6 of the 8 statements the change touches; 1 changed Go file(s) have no statement in the profile: `c.go` |",
		"| `//nolint` added | 1 |", "| TODO comments added | 2 |", "| Dependencies added | `example.com/dep v1.0.0` |",
		"| `a.go` | 400 | 480 | 500 |", "| `b.go` | new | 400 | 500 |",
	} {
		assert.Contains(t, out, want)
	}

	out = Markdown(Metrics{From: "1", Head: "2", Coverage: &Coverage{}})
	assert.Contains(t, out, "| Diff coverage | no changed statement in the profile |")
	assert.Contains(t, out, "| Dependencies added | none |")
	assert.Contains(t, out, "No code file it lengthens ends within a quarter of its size limit.")

	out = Markdown(Metrics{})
	assert.Contains(t, out, "| Diff coverage | not measured: `make pr` kept no coverage profile for this commit |")
}
