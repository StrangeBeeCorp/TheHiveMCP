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
