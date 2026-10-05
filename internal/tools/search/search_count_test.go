package search_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestSearchCasesWithCountOnly(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestCase(t, hiveClient, "High severity case 1", 3, "New", "")
	createTestCase(t, hiveClient, "High severity case 2", 3, "InProgress", "")
	createTestCase(t, hiveClient, "Low severity case", 1, "New", "")

	mcpClient := newSearchClient(t)

	structuredData := searchStructured(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeCase,
		pFilters: map[string]any{
			tGte: map[string]any{
				tField: tSeverity,
				tValue: 3,
			},
		},
		tCount: true,
	})

	countOnly, ok := structuredData["countOnly"].(bool)
	require.True(t, ok)
	require.True(t, countOnly)

	count, ok := structuredData[tCount].(float64)
	require.True(t, ok)
	require.InDelta(t, float64(2), count, 0)

	require.Equal(t, types.EntityTypeCase, structuredData["entityType"])
	require.NotNil(t, structuredData["rawFilters"])
}

func TestSearchAlertsWithCountOnly(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestAlert(t, hiveClient, "Critical Alert 1", 4, []string{tMalware, tPhishing})
	createTestAlert(t, hiveClient, "Critical Alert 2", 4, []string{tMalware})
	createTestAlert(t, hiveClient, "Medium Alert", 2, []string{"suspicious"})

	mcpClient := newSearchClient(t)

	structuredData := searchStructured(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeAlert,
		pFilters: map[string]any{
			tEq: map[string]any{
				tField: tSeverity,
				tValue: 4,
			},
		},
		tCount: true,
	})

	countOnly, ok := structuredData["countOnly"].(bool)
	require.True(t, ok)
	require.True(t, countOnly)

	countValue, ok := structuredData[tCount].(float64)
	require.True(t, ok)
	require.InDelta(t, float64(2), countValue, 0)
	require.Equal(t, types.EntityTypeAlert, structuredData["entityType"])
}

func TestSearchCountVsRegularSearch(t *testing.T) {
	testutils.Parallel(t)

	hiveClient := testutils.SetupTestWithCleanup(t)

	createTestCase(t, hiveClient, "Test case 1", 2, "New", "")
	createTestCase(t, hiveClient, "Test case 2", 2, "InProgress", "")
	createTestCase(t, hiveClient, "Test case 3", 2, "New", "")

	mcpClient := newSearchClient(t)

	// Same filter, toggling only count, so the count must equal the row count.
	args := func(count bool) map[string]any {
		return map[string]any{
			pEntityType: types.EntityTypeCase,
			pFilters:    map[string]any{tEq: map[string]any{tField: tSeverity, tValue: 2}},
			tCount:      count,
		}
	}

	regularResults := searchRows(t, mcpClient, args(false))
	regularCount := len(regularResults)

	countData := searchStructured(t, mcpClient, args(true))

	countOnlyValue, ok := countData[tCount].(float64)
	require.True(t, ok)

	require.InDelta(t, float64(regularCount), countOnlyValue, 0)
	require.Equal(t, 3, regularCount) // We created 3 test cases
}
