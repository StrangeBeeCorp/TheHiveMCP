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

// Cortex jobs and actions declare startDate, not _createdAt, as their sortable
// date field: defaulting them to _createdAt would make every unsorted search
// fail on a field TheHive does not expose for those types.
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
