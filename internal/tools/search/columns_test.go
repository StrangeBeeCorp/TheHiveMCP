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

// extra-columns is a projection, not an addition: it replaces the defaults.
// The name says otherwise and has misled a caller, so pin the real contract
// here next to the description that warns about it.
func TestExtraColumns_ReplaceTheDefaults(t *testing.T) {
	t.Parallel()

	tool := &Tool{}
	params := EntitiesParams{EntityType: types.EntityTypeAlert, ExtraColumns: []string{fieldTags}}

	require.NoError(t, tool.ValidateParams(&params))

	assert.Equal(t, []string{fieldTags}, params.ExtraColumns,
		"a requested column set is returned verbatim; defaults apply only when none is given")
}

// Omitting it keeps the entity defaults.
func TestExtraColumns_OmittedKeepsDefaults(t *testing.T) {
	t.Parallel()

	tool := &Tool{}
	params := EntitiesParams{EntityType: types.EntityTypeAlert}

	require.NoError(t, tool.ValidateParams(&params))

	assert.Equal(t, types.DefaultFields[types.EntityTypeAlert], params.ExtraColumns)
}
