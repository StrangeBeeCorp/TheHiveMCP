package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// fieldTags is used across these cases as a representative non-default column.
const fieldTags = "tags"

// An observable row without its value identifies a type and a timestamp but not
// the indicator, which is the whole point of the query. It is also the most
// common SOC search, and the omission is silent: the caller gets rows back and
// has to guess that extra-columns=["data"] exists.
func TestDefaultColumns_ObservablesCarryTheirValue(t *testing.T) {
	t.Parallel()

	tool := &Tool{}
	params := EntitiesParams{EntityType: types.EntityTypeObservable}

	require.NoError(t, tool.ValidateParams(&params))
	assert.Contains(t, params.ExtraColumns, "data", "an observable must return the IOC itself")
	assert.Contains(t, params.ExtraColumns, "dataType")
}

// extra-columns adds to the defaults, so asking for one more column cannot cost
// you title, severity or status.
func TestExtraColumns_ExtendTheDefaults(t *testing.T) {
	t.Parallel()

	tool := &Tool{}

	t.Run("defaults survive alongside the requested column", func(t *testing.T) {
		t.Parallel()

		params := EntitiesParams{EntityType: types.EntityTypeAlert, ExtraColumns: []string{fieldTags}}
		require.NoError(t, tool.ValidateParams(&params))

		for _, column := range types.DefaultFields[types.EntityTypeAlert] {
			assert.Contains(t, params.ExtraColumns, column, "requesting a column must not drop a default")
		}

		assert.Contains(t, params.ExtraColumns, fieldTags)
	})

	t.Run("a column named twice is projected once", func(t *testing.T) {
		t.Parallel()

		params := EntitiesParams{EntityType: types.EntityTypeAlert, ExtraColumns: []string{fieldTitle, fieldTags, fieldTags}}
		require.NoError(t, tool.ValidateParams(&params))

		assert.Equal(t, 1, countOccurrences(params.ExtraColumns, fieldTitle))
		assert.Equal(t, 1, countOccurrences(params.ExtraColumns, fieldTags))
	})

	t.Run("omitted keeps exactly the defaults", func(t *testing.T) {
		t.Parallel()

		params := EntitiesParams{EntityType: types.EntityTypeAlert}
		require.NoError(t, tool.ValidateParams(&params))

		assert.Equal(t, types.DefaultFields[types.EntityTypeAlert], params.ExtraColumns)
	})
}

func countOccurrences(values []string, want string) int {
	found := 0

	for _, value := range values {
		if value == want {
			found++
		}
	}

	return found
}
