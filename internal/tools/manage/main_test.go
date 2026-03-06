package manage_test

import (
	"context"
	"os"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutils.TeardownContainers(context.Background())
	os.Exit(code)
}
