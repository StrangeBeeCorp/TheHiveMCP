package analyzers_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/analyzers"
)

// The packages under testdata/src stand in for the client, internal/utils, internal/tools and
// internal/testutils; each `// want` comment is a report the analyzer must make, and every line
// without one must stay quiet.

func TestUntrustedFindsEntityTextInServerStrings(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), analyzers.Untrusted, "github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/handler")
}

func TestCutFlagFindsResultsThatHideACut(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), analyzers.CutFlag, "cut")
}

func TestParallelFindsIntegrationTestsOutOfOrder(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), analyzers.Parallel, "github.com/StrangeBeeCorp/TheHiveMCP/internal/integ")
}
