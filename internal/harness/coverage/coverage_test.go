package coverage_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/coverage"
)

const (
	module  = "example.com/m"
	utils   = "internal/utils"
	tools   = "internal/tools"
	profile = `mode: atomic
example.com/m/internal/utils/a.go:1.1,3.2 3 1
example.com/m/internal/utils/a.go:4.1,5.2 1 0
example.com/m/internal/utils/b.go:1.1,2.2 4 2
example.com/m/internal/tools/t.go:1.1,2.2 2 0
example.com/m/internal/tools/t.go:1.1,2.2 2 5
example.com/m/internal/tools/t.go:3.1,9.2 8 0
example.com/m/cmd/x/main.go:1.1,2.2 5 0
`
)

func measured(t *testing.T) map[string]coverage.Package {
	t.Helper()

	pkgs, err := coverage.Parse(strings.NewReader(profile), module)
	require.NoError(t, err)

	return pkgs
}

func TestAProfileIsSummedPerPackage(t *testing.T) {
	t.Parallel()

	pkgs := measured(t)

	assert.Equal(t, coverage.Package{Statements: 8, Covered: 7}, pkgs[utils])
	assert.Equal(t, coverage.Package{Statements: 10, Covered: 2}, pkgs[tools], "a block run by any binary is covered once")
	assert.InDelta(t, 87.5, pkgs[utils].Percent(), 0.01)
	assert.Zero(t, pkgs["cmd/x"].Percent())

	_, err := coverage.Parse(strings.NewReader("mode: set\nnot a block\n"), module)
	require.ErrorIs(t, err, coverage.ErrProfile)
}

func TestAPackageUnderItsFloorFails(t *testing.T) {
	t.Parallel()

	pkgs := measured(t)

	assert.Empty(t, coverage.Check(pkgs, coverage.Floors{Floors: map[string]float64{utils: 87, tools: 20}}))

	problems := coverage.Check(pkgs, coverage.Floors{Floors: map[string]float64{utils: 90, "internal/gone": 50}})
	require.Len(t, problems, 2)
	assert.Contains(t, problems[0], "internal/gone: has a floor of 50.0% but no coverage")
	assert.Contains(t, problems[1], "internal/utils: 87.5% of 8 statements, under its floor of 90.0%")
}

func TestFloorsAreProposedFromTheMeasurement(t *testing.T) {
	t.Parallel()

	floors := coverage.Propose(measured(t))

	assert.Equal(t, map[string]float64{utils: 86, tools: 19}, floors.Floors, "a point of slack, rounded down; untested packages get none")
	assert.Empty(t, coverage.Check(measured(t), floors), "the proposal holds against its own measurement")
	assert.Equal(t, "3 packages measured, 2 floors held, 0 tested packages without a floor", coverage.Summary(measured(t), floors))
}
