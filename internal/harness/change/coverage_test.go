package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mod = "example.com/m"

// A block measured by several test binaries is one block, covered if any of them ran it.
func TestTheProfileCountsABlockOnceCoveredIfAnyRunCoveredIt(t *testing.T) {
	t.Parallel()

	blocks, err := ParseProfile([]byte(`mode: atomic
example.com/m/a/a.go:3.14,5.2 2 0
example.com/m/a/a.go:3.14,5.2 2 4

example.com/m/a/a.go:7.10,9.2 1 0
`), mod)
	require.NoError(t, err)
	assert.Equal(t, []Block{
		{File: "a/a.go", Start: 3, End: 5, Statements: 2, Covered: true},
		{File: "a/a.go", Start: 7, End: 9, Statements: 1},
	}, blocks)
}

func TestAProfileItCannotReadIsAnError(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"a.go:3.14,5.2 2", "a.go 2 1", "a.go:3.14 2 1", "a.go:x.1,5.2 2 1", "a.go:3.1,y.2 2 1", "a.go:3.1,5.2 z 1",
		"a.go:3.1,5.2 2 z",
	} {
		_, err := ParseProfile([]byte("mode: set\n"+line+"\n"), mod)
		require.ErrorIs(t, err, ErrProfile, line)
	}
}

// Diff coverage counts the statements of the blocks a changed line falls in, each block once, in
// the change's non-test Go files; a changed file the profile does not measure is named.
func TestDiffCoverageCountsTheBlocksAChangedLineFallsIn(t *testing.T) {
	t.Parallel()

	blocks := []Block{
		{File: aGoFile, Start: 3, End: 5, Statements: 2, Covered: true},
		{File: aGoFile, Start: 7, End: 9, Statements: 3},
		{File: aGoFile, Start: 11, End: 12, Statements: 4},
	}
	d := Diff{
		aGoFile:     {{N: 3}, {N: 4}, {N: 8}, {N: 10}},
		"a_test.go": {{N: 1}},
		"README.md": {{N: 1}},
		"z.go":      {{N: 1}},
		bGoFile:     {{N: 1}},
		"empty.go":  {},
	}

	c := DiffCoverage(d, blocks)
	assert.Equal(t, Coverage{Statements: 5, Covered: 2, Unmeasured: []string{bGoFile, "z.go"}}, c)
	assert.InDelta(t, 40.0, c.Percent(), 0.01)
	assert.Zero(t, Coverage{}.Percent(), "no statement: no share")
}
