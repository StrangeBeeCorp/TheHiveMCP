package search_test

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/search"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// searchRowsAnswered runs search-entities against a TheHive that answers rows
// as it sent them, whatever the exclusion list of the query asked.
func searchRowsAnswered(t *testing.T, rows string, params search.EntitiesParams) []map[string]any {
	t.Helper()

	hive := &testutils.FakeQuery{Reply: testutils.Answer{Body: rows}}

	ctx := utils.AddPermissionsToContext(context.Background(), permissions.LoadAdminForTesting())
	ctx = context.WithValue(ctx, types.HiveClientCtxKey, hive.Client(t))

	tool := &search.Tool{}
	require.NoError(t, tool.ValidateParams(&params))

	result, err := tool.Handle(ctx, mcp.CallToolRequest{}, params)
	require.NoError(t, err)

	return result.Results
}

// TheHive 5.8 returns a task's timeToHandle whatever columns were asked: the
// exclusion list TheHive receives comes from the client's model, which predates
// the field. The rows must carry what the caller asked for, nothing else.
//
// spec: VER-002
func TestSearchDropsTheTaskFieldsTheClientModelDoesNotList(t *testing.T) {
	t.Parallel()

	rows := searchRowsAnswered(t, testutils.RecordedAnswer(t, "query-task-rows.json"), search.EntitiesParams{EntityType: types.EntityTypeTask})

	require.Len(t, rows, 2)

	for _, row := range rows {
		assert.NotContains(t, row, "timeToHandle", "a field the task model does not list was not asked for")
		assert.Contains(t, row, "_id")
		assert.Contains(t, row, "title")
		assert.Contains(t, row, "status")
	}
}

// spec: VER-002
func TestSearchDropsTheActionExtraDataNobodyAskedFor(t *testing.T) {
	t.Parallel()

	rows := searchRowsAnswered(t, testutils.RecordedAnswer(t, "query-action-rows.json"), search.EntitiesParams{EntityType: types.EntityTypeAction})

	require.Len(t, rows, 1)
	assert.NotContains(t, rows[0], "extraData")
	assert.Contains(t, rows[0], "_id")
	assert.Contains(t, rows[0], "responderName")
	assert.Contains(t, rows[0], "objectId")
}

// A column the caller asked for stays, whether or not the client model lists it.
func TestSearchKeepsAnUnlistedFieldTheCallerAskedForInExtraColumns(t *testing.T) {
	t.Parallel()

	rows := searchRowsAnswered(t, testutils.RecordedAnswer(t, "query-task-rows.json"), search.EntitiesParams{
		EntityType:   types.EntityTypeTask,
		ExtraColumns: []string{"timeToHandle"},
	})

	require.Len(t, rows, 2)

	for _, row := range rows {
		assert.Contains(t, row, "timeToHandle")
		assert.Contains(t, row, "_id")
	}
}
