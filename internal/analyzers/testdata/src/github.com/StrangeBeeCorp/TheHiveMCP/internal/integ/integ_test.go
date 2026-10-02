package integ

import (
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
)

func TestReachesTheHiveInParallel(t *testing.T) {
	t.Parallel() // want `TestReachesTheHiveInParallel reaches TheHive through testutils`

	_ = testutils.SetupTestWithCleanup(t)
}

func TestReachesTheHiveInOrder(t *testing.T) {
	testutils.Parallel(t)

	_ = testutils.SetupTestWithCleanup(t)
}

func TestReachesTheHiveWithParallelSubtests(t *testing.T) {
	testutils.Parallel(t)

	t.Run("one", func(t *testing.T) {
		t.Parallel()

		_ = testutils.SetupTestWithCleanup(t)
	})
}

func TestPureUnit(t *testing.T) {
	t.Parallel()

	_ = testutils.MockInputAlert()
}

func TestReachesTheHiveOnlyInASubtest(t *testing.T) {
	t.Parallel() // want `TestReachesTheHiveOnlyInASubtest reaches TheHive through testutils`

	t.Run("one", func(t *testing.T) {
		_ = testutils.SetupTestWithCleanup(t)
	})
}

// A helper is not a test: its caller decides how it runs.
func reachTheHive(t *testing.T) {
	t.Parallel()

	_ = testutils.SetupTestWithCleanup(t)
}

func TestThroughAHelperOutOfSight(t *testing.T) {
	t.Parallel() // the helper's call to testutils is not followed

	reachTheHive(t)
}
