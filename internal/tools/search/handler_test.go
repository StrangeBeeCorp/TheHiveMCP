package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		types.EntityTypeJob,
		types.EntityTypeAction,
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

// TheHive attaches an analyzer report to every job row regardless of extra-data,
// and exclude_fields does not suppress it. It is bulky third-party content, so it
// is dropped unless the caller asked for it.
func TestDropUnrequestedJobReport(t *testing.T) {
	t.Parallel()

	jobRow := func() map[string]any {
		return map[string]any{
			"_id": "~1",
			"extraData": map[string]any{
				"report": map[string]any{"artifacts": []any{"observable"}},
			},
		}
	}

	t.Run("dropped when not requested", func(t *testing.T) {
		t.Parallel()

		results := []map[string]any{jobRow()}
		dropUnrequestedJobReport(EntitiesParams{EntityType: types.EntityTypeJob}, results)

		assert.NotContains(t, results[0], "extraData", "an extraData holding only the report is removed entirely")
	})

	t.Run("kept when requested", func(t *testing.T) {
		t.Parallel()

		results := []map[string]any{jobRow()}
		dropUnrequestedJobReport(EntitiesParams{EntityType: types.EntityTypeJob, ExtraData: []string{"report"}}, results)

		assert.Contains(t, results[0]["extraData"], "report")
	})

	t.Run("other extraData keys survive", func(t *testing.T) {
		t.Parallel()

		row := jobRow()
		rowExtraData, ok := row["extraData"].(map[string]any)
		require.True(t, ok)

		rowExtraData["links"] = "kept"
		results := []map[string]any{row}

		dropUnrequestedJobReport(EntitiesParams{EntityType: types.EntityTypeJob}, results)

		extraData, ok := results[0]["extraData"].(map[string]any)
		require.True(t, ok)
		assert.NotContains(t, extraData, "report")
		assert.Contains(t, extraData, "links")
	})

	t.Run("other entity types untouched", func(t *testing.T) {
		t.Parallel()

		results := []map[string]any{jobRow()}
		dropUnrequestedJobReport(EntitiesParams{EntityType: types.EntityTypeObservable}, results)

		assert.Contains(t, results[0]["extraData"], "report")
	})
}
