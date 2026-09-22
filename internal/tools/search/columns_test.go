package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// fieldTags is used across these cases as a representative non-default column.
const fieldTags = "tags"

// fieldPlatforms is the pattern's opt-in platform list.
const fieldPlatforms = "platforms"

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

// A pattern's default row answers "which technique is this?" — its id, name and
// tactics. platforms is a four-to-six element list on most techniques, so
// carrying it by default triples the width of a catalog page for a detail few
// queries ask about. It stays reachable through extra-columns.
func TestDefaultColumns_PatternsOmitPlatforms(t *testing.T) {
	t.Parallel()

	tool := &Tool{}
	params := EntitiesParams{EntityType: types.EntityTypePattern}
	require.NoError(t, tool.ValidateParams(&params))

	assert.NotContains(t, params.ExtraColumns, fieldPlatforms,
		"platforms must be opt-in, not a default pattern column")

	for _, column := range []string{fieldID, "patternId", fieldName, "tactics"} {
		assert.Contains(t, params.ExtraColumns, column, "a pattern row must still identify its technique")
	}

	optedIn := EntitiesParams{EntityType: types.EntityTypePattern, ExtraColumns: []string{fieldPlatforms}}
	require.NoError(t, tool.ValidateParams(&optedIn))
	assert.Contains(t, optedIn.ExtraColumns, fieldPlatforms,
		"extra-columns=[platforms] must bring the list back")
}
