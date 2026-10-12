package analyzers_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
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

// The names the analyzer trusts come sorted, for internal/utils to compare with the wrapper's, and
// they are identifiers and statuses, never free text.
func TestTrustedFieldsAreSortedIdentifiers(t *testing.T) {
	t.Parallel()

	names := analyzers.TrustedFields()
	assert.True(t, slices.IsSorted(names), "sorted")

	for _, name := range []string{"_id", "_createdBy", "status"} {
		assert.Contains(t, names, name)
	}

	for _, name := range []string{"title", "description", "message"} {
		assert.NotContains(t, names, name)
	}
}

func TestCutFlagFindsResultsThatHideACut(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), analyzers.CutFlag, "cut")
}

func TestParallelFindsIntegrationTestsOutOfOrder(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), analyzers.Parallel, "github.com/StrangeBeeCorp/TheHiveMCP/internal/integ")
}
