package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/search"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

const sealedJobRows = `[` +
	`{"_id":"~1","analyzerName":"A_1_0","extraData":{"report":{"status":"ignore me"},"links":["x"]}},` +
	`{"_id":"~2","analyzerName":"B_1_0","extraData":{"report":{"summary":"s"}}},` +
	`{"_id":"~3","analyzerName":"C_1_0"}` +
	`]`

// Every row of a page follows the same rule, whatever extraData it carries, and a
// request for another extra-data key neither brings the report back nor loses
// the key asked for.
//
// spec: VER-002
func TestSearchJobsRowsFollowExtraDataAskedForRowByRow(t *testing.T) {
	t.Parallel()

	t.Run("none asked: no row carries extraData", func(t *testing.T) {
		t.Parallel()

		rows := searchRowsAnswered(t, sealedJobRows, search.EntitiesParams{EntityType: types.EntityTypeJob})

		require.Len(t, rows, 3)

		for _, row := range rows {
			assert.NotContains(t, row, "extraData")
			assert.Contains(t, row, "analyzerName")
		}
	})

	t.Run("links asked: the report stays out, links stay", func(t *testing.T) {
		t.Parallel()

		rows := searchRowsAnswered(t, sealedJobRows, search.EntitiesParams{EntityType: types.EntityTypeJob, ExtraData: []string{"links"}})

		require.Len(t, rows, 3)

		first, isObject := rows[0]["extraData"].(map[string]any)
		require.True(t, isObject)
		assert.NotContains(t, first, "report")
		assert.Contains(t, first, "links")
		assert.NotContains(t, rows[1], "extraData", "a row whose extraData held only the report has none left")
		assert.NotContains(t, rows[2], "extraData")
	})

	t.Run("report asked: every report is untrusted", func(t *testing.T) {
		t.Parallel()

		rows := searchRowsAnswered(t, sealedJobRows, search.EntitiesParams{EntityType: types.EntityTypeJob, ExtraData: []string{"report"}})

		require.Len(t, rows, 3)

		for _, row := range rows[:2] {
			extraData, isObject := row["extraData"].(map[string]any)
			require.True(t, isObject)
			assert.IsType(t, utils.UntrustedSubtree{}, extraData["report"])
		}

		assert.NotContains(t, rows[2], "extraData")
	})
}
