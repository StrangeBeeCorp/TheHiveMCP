package search_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	var b strings.Builder

	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// Exercises every operator of the TheHive filter DSL (hive://docs/overview/filter-dsl).
func TestFilterOperators(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Severities 1..4 = Low..Critical.
	a1 := createTestAlert(t, hiveClient, "Phishing Campaign", 4, []string{tPhishing, "email"})
	createTestAlert(t, hiveClient, "Malware Detected", 3, []string{tMalware, "endpoint"})
	createTestAlert(t, hiveClient, "Network Scan", 2, []string{tNetwork})
	createTestAlert(t, hiveClient, "Low Noise Event", 1, []string{"noise"})

	mcpClient := newSearchClient(t)

	// A nil filter is omitted entirely (match-all).
	run := func(t *testing.T, filter map[string]any) []any {
		t.Helper()

		args := map[string]any{
			pEntityType:   types.EntityTypeAlert,
			pExtraColumns: []string{tID, tTitle, tSeverity, tTags},
			"limit":       100,
		}
		if filter != nil {
			args[pFilters] = filter
		}

		return searchRows(t, mcpClient, args)
	}

	a1ID, ok := a1[tID].(string)
	require.True(t, ok)

	cases := []struct {
		name   string
		filter map[string]any
		want   int
	}{
		{tEq, map[string]any{tEq: map[string]any{tField: tSeverity, tValue: 4}}, 1},
		{"_ne", map[string]any{"_ne": map[string]any{tField: tSeverity, tValue: 4}}, 3},
		{"_gt", map[string]any{"_gt": map[string]any{tField: tSeverity, tValue: 3}}, 1},
		{tGte, map[string]any{tGte: map[string]any{tField: tSeverity, tValue: 3}}, 2},
		{"_lt", map[string]any{"_lt": map[string]any{tField: tSeverity, tValue: 2}}, 1},
		{tLte, map[string]any{tLte: map[string]any{tField: tSeverity, tValue: 2}}, 2},
		// _between is half-open: _from=2,_to=4 matches severities 2 and 3, not 4.
		{tBetween, map[string]any{tBetween: map[string]any{tField: tSeverity, "_from": 2, "_to": 4}}, 2},
		{tIn, map[string]any{tIn: map[string]any{tField: tSeverity, tValues: []any{1, 4}}}, 2},
		// _contains takes a bare field name and matches entities that have it set.
		{"_contains", map[string]any{"_contains": tTitle}, 4},
		// _like uses * wildcards, case-insensitive.
		{tLike, map[string]any{tLike: map[string]any{tField: tTitle, tValue: "*Malware*"}}, 1},
		{"_startsWith", map[string]any{"_startsWith": map[string]any{tField: tTitle, tValue: "Network"}}, 1},
		{"_endsWith", map[string]any{"_endsWith": map[string]any{tField: tTitle, tValue: "Detected"}}, 1},
		// _match is a full-text token match on the analyzed text field.
		{"_match", map[string]any{"_match": map[string]any{tField: tTitle, tValue: "Campaign"}}, 1},
		{tID, map[string]any{tID: a1ID}, 1},
		{"_not", map[string]any{"_not": map[string]any{tEq: map[string]any{tField: tSeverity, tValue: 1}}}, 3},
		// Date fields accept ISO 8601 strings (converted to timestamps); all 4 are recent.
		{"iso_date_value", map[string]any{tGte: map[string]any{tField: tCreatedAt, tValue: "2020-01-01T00:00:00"}}, 4},
		{"_or_of_like", map[string]any{tOr: []any{
			map[string]any{tLike: map[string]any{tField: tTitle, tValue: "*malware*"}},
			map[string]any{tLike: map[string]any{tField: tTitle, tValue: "*phishing*"}},
		}}, 2},
		{tAnd, map[string]any{tAnd: []any{
			map[string]any{tGte: map[string]any{tField: tSeverity, tValue: 3}},
			map[string]any{tIn: map[string]any{tField: tTags, tValues: []any{tPhishing}}},
		}}, 1},
		{tOr, map[string]any{tOr: []any{
			map[string]any{tEq: map[string]any{tField: tSeverity, tValue: 1}},
			map[string]any{tEq: map[string]any{tField: tSeverity, tValue: 4}},
		}}, 2},
		// Matches Phishing Campaign + Network Scan.
		{"complex_nested", map[string]any{tAnd: []any{
			map[string]any{tGte: map[string]any{tField: tSeverity, tValue: 2}},
			map[string]any{tOr: []any{
				map[string]any{tIn: map[string]any{tField: tTags, tValues: []any{tPhishing}}},
				map[string]any{tIn: map[string]any{tField: tTags, tValues: []any{tNetwork}}},
			}},
		}}, 2},
		{"_any_explicit", map[string]any{"_any": map[string]any{}}, 4},
		{"omitted_filters_match_all", nil, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Len(t, run(t, tc.filter), tc.want)
		})
	}
}

// A filter on a non-existent field must return an actionable error hinting at the
// schema resources (self-correct, no inner retry).
func TestSearchInvalidFieldReturnsActionableError(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	createTestAlert(t, hiveClient, "Some Alert", 2, []string{"test"})

	mcpClient := newSearchClient(t)

	result := callSearch(t, mcpClient, map[string]any{
		pEntityType: types.EntityTypeAlert,
		pFilters:    map[string]any{tEq: map[string]any{tField: "thisFieldDoesNotExist", tValue: "x"}},
	})
	require.True(t, result.IsError, "searching on a non-existent field should return a tool error")

	text := resultText(t, result)
	require.Contains(t, text, "hive://schema/", "the error hint should point at the schema resources for self-correction")
}
