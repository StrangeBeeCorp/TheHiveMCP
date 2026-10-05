package mutation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/harness/mutation"
)

const (
	utils  = "internal/utils"
	policy = "internal/harness/policy"
)

var config = mutation.Config{Tool: "example.com/gremlins@v1.0.0", TimeoutCoefficient: 30, Floors: map[string]float64{utils: 80, policy: 70}}

func TestTheChangedPackagesWithAFloorAreMutated(t *testing.T) {
	t.Parallel()

	floored, unfloored := config.Packages([]string{
		"internal/utils/parsers.go", "internal/utils/fuzz_test.go", "internal/harness/policy/paths.go",
		"internal/tools/search/handler.go", "internal/utils/testdata/x.go", "README.md", "gates/mutation.yaml",
	})

	assert.Equal(t, []string{policy, utils}, floored, "each package once, its tests included")
	assert.Equal(t, []string{"internal/tools/search"}, unfloored)
	assert.Equal(t, []string{"run", "example.com/gremlins@v1.0.0", "unleash", "--timeout-coefficient", "30", "--output", "out.json", "./" + utils},
		config.Args(utils, "out.json"))
}

func TestAPackageUnderItsFloorFails(t *testing.T) {
	t.Parallel()

	report, err := mutation.ParseReport([]byte(`{"test_efficacy": 82.98, "mutations_coverage": 78.33, "mutants_killed": 78, "mutants_lived": 16,
		"mutants_not_covered": 26, "mutator_statistics": {"conditionals_negation": 101, "arithmetic_base": 20}}`))
	require.NoError(t, err)

	line, ok := config.Check(utils, report)
	assert.True(t, ok)
	assert.Equal(t, "internal/utils: efficacy 83.0% (floor 80%), 78 killed, 16 lived, 1 timed out, 26 not covered", line)

	report, err = mutation.ParseReport([]byte(`{"test_efficacy": 65, "mutants_killed": 13, "mutants_lived": 7,
		"mutator_statistics": {"conditionals_negation": 20},
		"files": [{"file_name": "paths.go", "mutations": [{"type": "CONDITIONALS_NEGATION", "status": "LIVED", "line": 12},
			{"type": "ARITHMETIC_BASE", "status": "KILLED", "line": 3}]}]}`))
	require.NoError(t, err)

	line, ok = config.Check(policy, report)
	assert.False(t, ok)
	assert.Contains(t, line, "add the tests that kill the mutants that lived: paths.go:12 CONDITIONALS_NEGATION")

	_, ok = config.Check(policy, mutation.Report{})
	assert.True(t, ok, "no mutant to kill: nothing to hold the package to")

	line, ok = config.Check(policy, mutation.Report{NotCovered: 12, Statistics: map[string]int{"conditionals_negation": 12}})
	assert.False(t, ok, "mutants, and no test reaching them")
	assert.Contains(t, line, "no unit test reaches its code")

	timedOut := mutation.Report{Efficacy: 90.9, Killed: 10, Lived: 1, NotCovered: 26, Statistics: map[string]int{"conditionals_negation": 121}}
	line, ok = config.Check(utils, timedOut)
	assert.False(t, ok, "84 of 95 covered mutants timed out: 90.9 % of the 11 left says little")
	assert.Contains(t, line, "84 timed out")
	assert.Contains(t, line, "raise timeout_coefficient")

	_, err = mutation.ParseReport([]byte("{"))
	require.Error(t, err)
}

func TestTheRepositoryConfigLoads(t *testing.T) {
	t.Parallel()

	c, err := mutation.Load(filepath.Join("..", "..", "..", "gates", "mutation.yaml"))
	require.NoError(t, err)
	assert.Contains(t, c.Tool, "gremlins@v")
	assert.Contains(t, c.Floored(), utils, "the trust boundary has a floor")

	bad := filepath.Join(t.TempDir(), "mutation.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("tool: gremlins\nfloors: {}\n"), 0o600))

	_, err = mutation.Load(bad)
	require.ErrorIs(t, err, mutation.ErrConfig)
}
