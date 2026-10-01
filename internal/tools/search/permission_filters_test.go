package search

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// Regression: the query embedded the permission filters by reference, then
// repaired keys and translated dates in place, so the first search rewrote the
// session's permission config, and concurrent searches raced on its maps.
func TestBuildHiveQueryLeavesThePermissionFiltersUntouched(t *testing.T) {
	t.Parallel()

	newPermFilters := func() map[string]any {
		return map[string]any{"_and": []any{
			map[string]any{"_gte": map[string]any{keyField: "_updatedAt", keyValue: "2024-01-01T00:00:00"}},
			map[string]any{`"_lte"`: map[string]any{keyField: "tlp", keyValue: 2}},
		}}
	}

	for name, userFilters := range map[string]map[string]any{
		"alone":            nil,
		"merged with user": {opEq: map[string]any{keyField: "status", keyValue: "New"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			permFilters := newPermFilters()
			merged, _ := permissions.MergeFilters(userFilters, permFilters)

			(&Tool{}).buildHiveQuery(EntitiesParams{EntityType: types.EntityTypeCase, Limit: 10}, merged)

			require.Equal(t, newPermFilters(), permFilters)
		})
	}
}
