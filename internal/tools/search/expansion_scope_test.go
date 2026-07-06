package search_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// TestExpandEntitiesWithQueriesScopeEnforcement verifies that additional-query
// expansion refuses to fetch children of a parent entity excluded by the
// configured permission filters, and proceeds unchanged when no filters are
// configured (DL-6004).
func TestExpandEntitiesWithQueriesScopeEnforcement(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	authContext := testutils.GetAuthContext(testutils.NewHiveTestConfig())
	ctx := context.WithValue(authContext, types.HiveClientCtxKey, hiveClient)

	// MockInputCase includes one task, so the tTasks expansion returns data
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

	permFilters := map[string]any{
		tLte: map[string]any{tField: "tlp", tValue: 2},
	}

	// Out-of-scope parent: expansion denied, no children fetched
	_, err = utils.ExpandEntitiesWithQueries(ctx, types.EntityTypeCase,
		[]map[string]any{{tID: outOfScopeCase.UnderscoreId}},
		[]string{tTasks}, permFilters)
	require.ErrorContains(t, err, "not within the scope")

	// In-scope parent: expansion succeeds and returns the case's tasks
	expanded, err := utils.ExpandEntitiesWithQueries(ctx, types.EntityTypeCase,
		[]map[string]any{{tID: inScopeCase.UnderscoreId}},
		[]string{tTasks}, permFilters)
	require.NoError(t, err)
	require.Len(t, expanded, 1)
	require.Contains(t, expanded[0], tTasks)
	require.NotEmpty(t, expanded[0][tTasks])

	// Without configured filters the same out-of-scope parent expands fine
	expanded, err = utils.ExpandEntitiesWithQueries(ctx, types.EntityTypeCase,
		[]map[string]any{{tID: outOfScopeCase.UnderscoreId}},
		[]string{tTasks}, nil)
	require.NoError(t, err)
	require.Len(t, expanded, 1)
	require.Contains(t, expanded[0], tTasks)
}
