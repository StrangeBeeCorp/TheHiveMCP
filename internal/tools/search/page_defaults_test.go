package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The entity types are spelled out rather than taken from the constants, and
// the test has a file of its own, so it builds on a tree without the
// knowledge-base type and fails there for the behaviour, not for a missing symbol.
func TestValidateParams_PageSearchesReturnTheirText(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	for _, entityType := range []string{"page", "knowledge-base"} {
		params := EntitiesParams{EntityType: entityType}
		require.NoError(t, tool.ValidateParams(&params), entityType)

		// A row that only names a page answers nothing: content is what a page is read for.
		assert.Subset(t, params.ExtraColumns, []string{fieldID, fieldTitle, "category", "content"}, entityType)
	}
}
