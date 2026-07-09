package search

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestGetExcludedFields_IdNeverExcluded(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	entityTypes := []string{
		types.EntityTypeAlert,
		types.EntityTypeCase,
		types.EntityTypeTask,
		types.EntityTypeObservable,
		types.EntityTypeProcedure,
		types.EntityTypePattern,
		types.EntityTypeCaseTemplate,
		types.EntityTypePage,
	}

	for _, entityType := range entityTypes {
		t.Run(entityType, func(t *testing.T) {
			t.Parallel()
			// When keptColumns does NOT include _id, it should still not be excluded
			excluded := tool.getExcludedFields(entityType, []string{fieldTitle}, nil)
			assert.NotContains(t, excluded, fieldID,
				"_id must never be excluded from search results for entity type %s", entityType)
		})
	}
}

func TestGetExcludedFields_IdNotExcludedWithExtraColumns(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	// Simulate the bug scenario: extra-columns specified without _id,
	// and additional-queries would need _id
	keptColumns := []string{fieldTitle, "description", "source", "tags", "severity"}
	excluded := tool.getExcludedFields(types.EntityTypeAlert, keptColumns, nil)

	assert.NotContains(t, excluded, fieldID,
		"_id must never be excluded even when not in keptColumns")
}

func TestGetExcludedFields_ExtraDataPreserved(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	// extraData should not be excluded when extraData list is non-empty
	excluded := tool.getExcludedFields(types.EntityTypeCase, []string{fieldTitle}, []string{"someField"})
	assert.NotContains(t, excluded, "extraData",
		"extraData should not be excluded when extraData list is non-empty")
}
