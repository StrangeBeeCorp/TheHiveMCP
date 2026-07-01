package search_test

import (
	"context"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/stretchr/testify/require"
)

// Additional-query expansion must not fetch children of a parent excluded by permission
// filters, and must proceed unchanged when no filters are configured (DL-6004).
func TestExpandEntitiesWithQueriesScopeEnforcement(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	ctx := context.WithValue(authContext, types.HiveClientCtxKey, hiveClient)

	// MockInputCase includes one task, so the "tasks" expansion returns data
	inScopeInput := testutils.MockInputCase()
	inScopeInput.Title = "In scope expansion case"
	inScopeCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*inScopeInput).Execute()
	require.NoError(t, err)

	outOfScopeInput := testutils.MockInputCase()
	outOfScopeInput.Title = "Out of scope expansion case"
	highTLP := int32(3)
	outOfScopeInput.Tlp = &highTLP
	outOfScopeCase, _, err := hiveClient.CaseAPI.CreateCase(authContext).InputCreateCase(*outOfScopeInput).Execute()
	require.NoError(t, err)

	permFilters := map[string]interface{}{
		"_lte": map[string]interface{}{"_field": "tlp", "_value": 2},
	}

	_, err = utils.ExpandEntitiesWithQueries(ctx, types.EntityTypeCase,
		[]map[string]interface{}{{"_id": outOfScopeCase.UnderscoreId}},
		[]string{"tasks"}, permFilters)
	require.ErrorContains(t, err, "not within the scope")

	expanded, err := utils.ExpandEntitiesWithQueries(ctx, types.EntityTypeCase,
		[]map[string]interface{}{{"_id": inScopeCase.UnderscoreId}},
		[]string{"tasks"}, permFilters)
	require.NoError(t, err)
	require.Len(t, expanded, 1)
	require.Contains(t, expanded[0], "tasks")
	require.NotEmpty(t, expanded[0]["tasks"])

	// No filters configured: the same out-of-scope parent expands (fail-open).
	expanded, err = utils.ExpandEntitiesWithQueries(ctx, types.EntityTypeCase,
		[]map[string]interface{}{{"_id": outOfScopeCase.UnderscoreId}},
		[]string{"tasks"}, nil)
	require.NoError(t, err)
	require.Len(t, expanded, 1)
	require.Contains(t, expanded[0], "tasks")
}
