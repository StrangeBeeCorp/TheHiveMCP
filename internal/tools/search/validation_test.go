package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestValidateParamsAcceptsAutomationEntityTypes(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	for _, entityType := range []string{types.EntityTypeJob, types.EntityTypeAction} {
		t.Run(entityType, func(t *testing.T) {
			t.Parallel()

			params := &EntitiesParams{EntityType: entityType}
			require.NoError(t, tool.ValidateParams(params))
			assert.Equal(t, types.DefaultFields[entityType], params.ExtraColumns)
		})
	}
}

// For a Cortex run, startDate is when the analyzer or responder actually ran, so
// it is what an unsorted search should order by.
func TestValidateParamsDefaultsSortFieldPerEntityType(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	sortFieldByEntityType := map[string]string{
		types.EntityTypeJob:    "startDate",
		types.EntityTypeAction: "startDate",
		types.EntityTypeCase:   "_createdAt",
		types.EntityTypeAlert:  "_createdAt",
	}

	for entityType, expectedSortField := range sortFieldByEntityType {
		t.Run(entityType, func(t *testing.T) {
			t.Parallel()

			params := &EntitiesParams{EntityType: entityType}
			require.NoError(t, tool.ValidateParams(params))
			assert.Equal(t, expectedSortField, params.SortBy)
		})
	}
}

func TestValidateParamsKeepsCallerSortField(t *testing.T) {
	t.Parallel()

	tool := &Tool{}
	params := &EntitiesParams{EntityType: types.EntityTypeJob, SortBy: "analyzerName"}

	require.NoError(t, tool.ValidateParams(params))
	assert.Equal(t, "analyzerName", params.SortBy)
}

func TestValidateParamsRejectsUnknownEntityType(t *testing.T) {
	t.Parallel()

	tool := &Tool{}
	err := tool.ValidateParams(&EntitiesParams{EntityType: "analyzer"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entity-type 'analyzer'")
}

// platforms is not indexed on Pattern: TheHive accepts a filter on it and
// returns zero rows, which a model reports as "no technique targets Linux".
func TestValidateParamsRejectsPatternPlatformFilter(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	filters := map[string]map[string]any{
		opEq:              {opEq: map[string]any{keyField: fieldPlatforms, keyValue: platformLinux}},
		"_contains":       {"_contains": fieldPlatforms},
		"nested in _and":  {"_and": []any{map[string]any{"_in": map[string]any{keyField: fieldPlatforms, keyValues: []any{platformLinux}}}}},
		"over-quoted key": {opEq: map[string]any{`"_field"`: fieldPlatforms, keyValue: platformLinux}},
	}

	for name, filter := range filters {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tool.ValidateParams(&EntitiesParams{EntityType: types.EntityTypePattern, Filters: filter})

			require.Error(t, err, "pattern filter %v tests platforms and must be rejected", filter)
			assert.Contains(t, err.Error(), "extra-columns", "the error must point at the way to read platforms")
		})
	}
}

// The ban is per entity and per field: platforms stays readable, name stays
// filterable, and no other entity is affected.
func TestValidateParamsAcceptsFiltersOnIndexedFields(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	accepted := map[string]*EntitiesParams{
		"pattern by name": {
			EntityType: types.EntityTypePattern,
			Filters:    map[string]any{"_like": map[string]any{keyField: "name", keyValue: "*Disk*"}},
		},
		"pattern platforms as a column": {
			EntityType:   types.EntityTypePattern,
			ExtraColumns: []string{fieldPlatforms},
			Filters:      map[string]any{opEq: map[string]any{keyField: fieldPatternID, keyValue: "T1561"}},
		},
		"platforms on another entity": {
			EntityType: types.EntityTypeCase,
			Filters:    map[string]any{opEq: map[string]any{keyField: fieldPlatforms, keyValue: platformLinux}},
		},
	}

	for name, params := range accepted {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.NoError(t, tool.ValidateParams(params))
		})
	}
}
