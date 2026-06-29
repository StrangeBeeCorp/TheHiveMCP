package search_test

import (
	"strings"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// resultText flattens a tool result's text content into a single string.
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

// TestFilterOperators exercises every operator of the TheHive filter DSL against
// a known dataset, so that every operator documented in the cheatsheet
// (hive://docs/overview/filter-dsl) and the tool description is verified to work
// end-to-end with no internal LLM. The dataset is created once and shared by the
// subtests (the test harness resets the instance only after the whole test).
//
// Notable semantics verified here: _between is half-open (_from <= field < _to,
// upper bound exclusive); _like uses * wildcards and is case-insensitive; _match
// is a full-text token match on analyzed text; and _contains takes a bare field
// name and tests that the field is present on the entity.
func TestFilterOperators(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Four alerts with controlled, distinct fields (severities 1..4 = Low..Critical).
	a1 := createTestAlert(t, hiveClient, "Phishing Campaign", 4, []string{"phishing", "email"})
	createTestAlert(t, hiveClient, "Malware Detected", 3, []string{"malware", "endpoint"})
	createTestAlert(t, hiveClient, "Network Scan", 2, []string{"network"})
	createTestAlert(t, hiveClient, "Low Noise Event", 1, []string{"noise"})

	mcpClient := newSearchClient(t)

	// run executes a search with the given filter and returns the result rows.
	// A nil filter is omitted entirely (match-all).
	run := func(t *testing.T, filter map[string]any) []any {
		t.Helper()
		args := map[string]any{
			"entity-type":   types.EntityTypeAlert,
			"extra-columns": []string{"_id", "title", "severity", "tags"},
			"limit":         100,
		}
		if filter != nil {
			args["filters"] = filter
		}
		return searchRows(t, mcpClient, args)
	}

	a1ID, ok := a1["_id"].(string)
	require.True(t, ok)

	cases := []struct {
		name   string
		filter map[string]any
		want   int
	}{
		{"_eq", map[string]any{"_eq": map[string]any{"_field": "severity", "_value": 4}}, 1},
		{"_ne", map[string]any{"_ne": map[string]any{"_field": "severity", "_value": 4}}, 3},
		{"_gt", map[string]any{"_gt": map[string]any{"_field": "severity", "_value": 3}}, 1},
		{"_gte", map[string]any{"_gte": map[string]any{"_field": "severity", "_value": 3}}, 2},
		{"_lt", map[string]any{"_lt": map[string]any{"_field": "severity", "_value": 2}}, 1},
		{"_lte", map[string]any{"_lte": map[string]any{"_field": "severity", "_value": 2}}, 2},
		// _between is half-open: _from=2,_to=4 matches severities 2 and 3, not 4.
		{"_between", map[string]any{"_between": map[string]any{"_field": "severity", "_from": 2, "_to": 4}}, 2},
		{"_in", map[string]any{"_in": map[string]any{"_field": "severity", "_values": []any{1, 4}}}, 2},
		// _contains takes a bare field name and matches entities that have it set.
		{"_contains", map[string]any{"_contains": "title"}, 4},
		// _like uses * wildcards, case-insensitive.
		{"_like", map[string]any{"_like": map[string]any{"_field": "title", "_value": "*Malware*"}}, 1},
		{"_startsWith", map[string]any{"_startsWith": map[string]any{"_field": "title", "_value": "Network"}}, 1},
		{"_endsWith", map[string]any{"_endsWith": map[string]any{"_field": "title", "_value": "Detected"}}, 1},
		// _match is a full-text token match on the analyzed text field.
		{"_match", map[string]any{"_match": map[string]any{"_field": "title", "_value": "Campaign"}}, 1},
		{"_id", map[string]any{"_id": a1ID}, 1},
		{"_not", map[string]any{"_not": map[string]any{"_eq": map[string]any{"_field": "severity", "_value": 1}}}, 3},
		// Date fields accept ISO 8601 strings (converted to timestamps); all 4 are recent.
		{"iso_date_value", map[string]any{"_gte": map[string]any{"_field": "_createdAt", "_value": "2020-01-01T00:00:00"}}, 4},
		// Documented example: title contains malware OR phishing (case-insensitive).
		{"_or_of_like", map[string]any{"_or": []any{
			map[string]any{"_like": map[string]any{"_field": "title", "_value": "*malware*"}},
			map[string]any{"_like": map[string]any{"_field": "title", "_value": "*phishing*"}},
		}}, 2},
		// severity >= 3 AND tagged phishing.
		{"_and", map[string]any{"_and": []any{
			map[string]any{"_gte": map[string]any{"_field": "severity", "_value": 3}},
			map[string]any{"_in": map[string]any{"_field": "tags", "_values": []any{"phishing"}}},
		}}, 1},
		// severity 1 OR severity 4.
		{"_or", map[string]any{"_or": []any{
			map[string]any{"_eq": map[string]any{"_field": "severity", "_value": 1}},
			map[string]any{"_eq": map[string]any{"_field": "severity", "_value": 4}},
		}}, 2},
		// severity >= 2 AND (tagged phishing OR network): Phishing Campaign + Network Scan.
		{"complex_nested", map[string]any{"_and": []any{
			map[string]any{"_gte": map[string]any{"_field": "severity", "_value": 2}},
			map[string]any{"_or": []any{
				map[string]any{"_in": map[string]any{"_field": "tags", "_values": []any{"phishing"}}},
				map[string]any{"_in": map[string]any{"_field": "tags", "_values": []any{"network"}}},
			}},
		}}, 2},
		// Explicit match-all form, and the omit-filters form.
		{"_any_explicit", map[string]any{"_any": map[string]any{}}, 4},
		{"omitted_filters_match_all", nil, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Len(t, run(t, tc.filter), tc.want)
		})
	}
}

// TestSearchInvalidFieldReturnsActionableError locks in the "self-correct, no
// inner retry" contract: when a filter references a field that does not exist,
// the tool returns an actionable error whose hint points the caller at the
// schema resources so it can correct the filter and call again.
func TestSearchInvalidFieldReturnsActionableError(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	createTestAlert(t, hiveClient, "Some Alert", 2, []string{"test"})

	mcpClient := newSearchClient(t)

	result := callSearch(t, mcpClient, map[string]any{
		"entity-type": types.EntityTypeAlert,
		"filters":     map[string]any{"_eq": map[string]any{"_field": "thisFieldDoesNotExist", "_value": "x"}},
	})
	require.True(t, result.IsError, "searching on a non-existent field should return a tool error")

	text := resultText(t, result)
	require.Contains(t, text, "hive://schema/", "the error hint should point at the schema resources for self-correction")
}
