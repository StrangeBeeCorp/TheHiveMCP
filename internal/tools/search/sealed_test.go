package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/search"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// Any field a client model does not list goes, not only the two TheHive 5.8
// added to tasks and actions: a future field of a case, an alert or an
// observable is no more asked for.
//
// spec: VER-002
func TestSearchDropsWhateverTheModelOfTheEntityDoesNotList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		entityType string
		row        string
		kept       []string
		dropped    []string
	}{
		{
			"case", types.EntityTypeCase,
			`{"_id":"~1","_createdAt":1791986090000,"title":"Phishing","status":"New","severity":2,"futureFlag":true,"anotherNewField":"x"}`,
			[]string{tID, "title", "status", "severity"}, []string{"futureFlag", "anotherNewField"},
		},
		{
			"alert", types.EntityTypeAlert,
			`{"_id":"~2","_createdAt":1791986090000,"title":"Alert","status":"New","severity":1,"someNewThing":{"a":1}}`,
			[]string{tID, "title", "severity"}, []string{"someNewThing"},
		},
		{
			"observable", types.EntityTypeObservable,
			`{"_id":"~3","_createdAt":1791986090000,"dataType":"ip","data":"192.0.2.1","brandNewField":1}`,
			[]string{tID, "dataType", "data"}, []string{"brandNewField"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rows := searchRowsAnswered(t, "["+tc.row+"]", search.EntitiesParams{EntityType: tc.entityType})
			require.Len(t, rows, 1)

			for _, field := range tc.kept {
				assert.Contains(t, rows[0], field)
			}

			for _, field := range tc.dropped {
				assert.NotContains(t, rows[0], field, "the model of a %s does not list it and nobody asked for it", tc.name)
			}
		})
	}
}

// A field outside the model stays as soon as the caller names it in
// extra-columns, and so does the extra data requested.
//
// spec: VER-002
func TestSearchKeepsWhatTheCallerAskedForWhateverTheModelLists(t *testing.T) {
	t.Parallel()

	row := `[{"_id":"~1","_createdAt":1791986090000,"title":"Phishing","status":"New","severity":2,"futureFlag":true,"description":"long text"}]`

	rows := searchRowsAnswered(t, row, search.EntitiesParams{EntityType: types.EntityTypeCase, ExtraColumns: []string{"futureFlag"}})
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "futureFlag", "asked for in extra-columns")

	action := `[{"_id":"~7","responderName":"r","status":"Success","objectType":"Case","objectId":"~1","extraData":{"note":"x"}}]`

	rows = searchRowsAnswered(t, action, search.EntitiesParams{EntityType: types.EntityTypeAction, ExtraData: []string{"note"}})
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0], "extraData", "extra-data was requested")
	assert.Contains(t, rows[0], "_id")
}
