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

// extraData is opt-in on a job row, and thehive.OutputJob does not list it: a job
// searched without extra-data carries none of it, whatever keys TheHive sent
// next to the report.
//
// spec: VER-002
func TestSearchJobRowCarriesNoExtraDataUnlessAskedFor(t *testing.T) {
	t.Parallel()

	const rows = `[{"_id":"~91","analyzerName":"VirusTotal_3_0","status":"Success",` +
		`"extraData":{"report":{"summary":"found"},"links":["https://example.test/a"]}}]`

	got := searchRowsAnswered(t, rows, search.EntitiesParams{EntityType: types.EntityTypeJob})

	require.Len(t, got, 1)
	assert.NotContains(t, got[0], "extraData", "extraData is opt-in through extra-data")
	assert.Contains(t, got[0], "_id")
	assert.Contains(t, got[0], "analyzerName")
}

// spec: VER-002
func TestSearchJobRowKeepsExtraDataAndTypesItsReportUntrustedWhenAskedFor(t *testing.T) {
	t.Parallel()

	const rows = `[{"_id":"~91","analyzerName":"VirusTotal_3_0","status":"Success",` +
		`"extraData":{"report":{"summary":"found"},"links":["https://example.test/a"]}}]`

	got := searchRowsAnswered(t, rows, search.EntitiesParams{EntityType: types.EntityTypeJob, ExtraData: []string{"report"}})

	require.Len(t, got, 1)

	extraData, isObject := got[0]["extraData"].(map[string]any)
	require.True(t, isObject, "extra-data [report] keeps extraData: %v", got[0])

	assert.IsType(t, utils.UntrustedSubtree{}, extraData["report"], "a Cortex report is third-party JSON")
	assert.Contains(t, extraData, "links", "the other keys of extraData stay")
}
