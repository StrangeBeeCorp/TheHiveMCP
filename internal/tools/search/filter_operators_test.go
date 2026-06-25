package search_test

import (
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// resultText flattens a tool result's text content into a single string.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var s string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			s += tc.Text + "\n"
		}
	}
	return s
}

// TestFilterOperators exercises every operator of the TheHive filter DSL against
// a known dataset, so that every operator documented in the cheatsheet
// (hive://docs/overview/filter-dsl) and the tool description is verified to work
// end-to-end with no internal LLM. The dataset is created once and shared by the
// subtests (the test harness resets the instance only after the whole test).
func TestFilterOperators(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)

	// Four alerts with controlled, distinct fields.
	a1 := createTestAlert(t, hiveClient, "Phishing Campaign", 4, []string{"phishing", "email"})
	createTestAlert(t, hiveClient, "Malware Detected", 3, []string{"malware", "endpoint"})
	createTestAlert(t, hiveClient, "Network Scan", 2, []string{"network"})
	createTestAlert(t, hiveClient, "Low Noise Event", 1, []string{"noise"})

	mcpClient := testutils.GetMCPTestClient(t, unusedSamplingHandler(t), testutils.DummyElicitationAccept)

	// run executes a search with the given filter and returns the result rows.
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
		result, err := mcpClient.CallTool(t.Context(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: "search-entities", Arguments: args},
		})
		require.NoError(t, err)
		require.False(t, result.IsError, "unexpected tool error: %s", resultText(t, result))
		data, ok := result.StructuredContent.(map[string]any)
		require.True(t, ok)
		rows, ok := data["results"].([]any)
		require.True(t, ok)
		return rows
	}

	t.Run("_eq", func(t *testing.T) {
		rows := run(t, map[string]any{"_eq": map[string]any{"_field": "severity", "_value": 4}})
		require.Len(t, rows, 1)
	})

	t.Run("_ne", func(t *testing.T) {
		rows := run(t, map[string]any{"_ne": map[string]any{"_field": "severity", "_value": 4}})
		require.Len(t, rows, 3)
	})

	t.Run("_gt", func(t *testing.T) {
		rows := run(t, map[string]any{"_gt": map[string]any{"_field": "severity", "_value": 3}})
		require.Len(t, rows, 1)
	})

	t.Run("_gte", func(t *testing.T) {
		rows := run(t, map[string]any{"_gte": map[string]any{"_field": "severity", "_value": 3}})
		require.Len(t, rows, 2)
	})

	t.Run("_lt", func(t *testing.T) {
		rows := run(t, map[string]any{"_lt": map[string]any{"_field": "severity", "_value": 2}})
		require.Len(t, rows, 1)
	})

	t.Run("_lte", func(t *testing.T) {
		rows := run(t, map[string]any{"_lte": map[string]any{"_field": "severity", "_value": 2}})
		require.Len(t, rows, 2)
	})

	t.Run("_between", func(t *testing.T) {
		// _between is half-open: _from <= field < _to (upper bound exclusive).
		// _from=2,_to=4 therefore matches severities 2 and 3, not 4.
		rows := run(t, map[string]any{"_between": map[string]any{"_field": "severity", "_from": 2, "_to": 4}})
		require.Len(t, rows, 2)
	})

	t.Run("_in", func(t *testing.T) {
		rows := run(t, map[string]any{"_in": map[string]any{"_field": "severity", "_values": []any{1, 4}}})
		require.Len(t, rows, 2)
	})

	t.Run("_contains", func(t *testing.T) {
		// _contains takes a bare string and tests whether the named field exists
		// (is present/non-empty) on the entity. All four alerts have a title.
		rows := run(t, map[string]any{"_contains": "title"})
		require.Len(t, rows, 4)
	})

	t.Run("_like", func(t *testing.T) {
		// _like uses * wildcards (TheHive query DSL), case-insensitive.
		rows := run(t, map[string]any{"_like": map[string]any{"_field": "title", "_value": "*Malware*"}})
		require.Len(t, rows, 1)
	})

	t.Run("_or_of_like", func(t *testing.T) {
		// Documented example: title contains malware OR phishing (case-insensitive).
		rows := run(t, map[string]any{"_or": []any{
			map[string]any{"_like": map[string]any{"_field": "title", "_value": "*malware*"}},
			map[string]any{"_like": map[string]any{"_field": "title", "_value": "*phishing*"}},
		}})
		require.Len(t, rows, 2)
	})

	t.Run("_startsWith", func(t *testing.T) {
		rows := run(t, map[string]any{"_startsWith": map[string]any{"_field": "title", "_value": "Network"}})
		require.Len(t, rows, 1)
	})

	t.Run("_endsWith", func(t *testing.T) {
		rows := run(t, map[string]any{"_endsWith": map[string]any{"_field": "title", "_value": "Detected"}})
		require.Len(t, rows, 1)
	})

	t.Run("_match", func(t *testing.T) {
		// _match is a full-text (analyzed) match on a text field.
		rows := run(t, map[string]any{"_match": map[string]any{"_field": "title", "_value": "Campaign"}})
		require.Len(t, rows, 1)
	})

	t.Run("_id", func(t *testing.T) {
		id, ok := a1["_id"].(string)
		require.True(t, ok)
		rows := run(t, map[string]any{"_id": id})
		require.Len(t, rows, 1)
	})

	t.Run("_and", func(t *testing.T) {
		rows := run(t, map[string]any{"_and": []any{
			map[string]any{"_gte": map[string]any{"_field": "severity", "_value": 3}},
			map[string]any{"_in": map[string]any{"_field": "tags", "_values": []any{"phishing"}}},
		}})
		require.Len(t, rows, 1)
	})

	t.Run("_or", func(t *testing.T) {
		rows := run(t, map[string]any{"_or": []any{
			map[string]any{"_eq": map[string]any{"_field": "severity", "_value": 1}},
			map[string]any{"_eq": map[string]any{"_field": "severity", "_value": 4}},
		}})
		require.Len(t, rows, 2)
	})

	t.Run("_not", func(t *testing.T) {
		rows := run(t, map[string]any{"_not": map[string]any{"_eq": map[string]any{"_field": "severity", "_value": 1}}})
		require.Len(t, rows, 3)
	})

	t.Run("iso_date_value", func(t *testing.T) {
		// Date fields accept ISO 8601 strings; the tool converts them to TheHive
		// timestamps before querying. All four alerts were created just now.
		rows := run(t, map[string]any{"_gte": map[string]any{"_field": "_createdAt", "_value": "2020-01-01T00:00:00"}})
		require.Len(t, rows, 4)
	})

	t.Run("complex_nested", func(t *testing.T) {
		// severity >= 2 AND (tagged phishing OR tagged network):
		// A1 (sev4, phishing) and Network Scan (sev2, network) match.
		rows := run(t, map[string]any{"_and": []any{
			map[string]any{"_gte": map[string]any{"_field": "severity", "_value": 2}},
			map[string]any{"_or": []any{
				map[string]any{"_in": map[string]any{"_field": "tags", "_values": []any{"phishing"}}},
				map[string]any{"_in": map[string]any{"_field": "tags", "_values": []any{"network"}}},
			}},
		}})
		require.Len(t, rows, 2)
	})

	t.Run("_any_explicit", func(t *testing.T) {
		// The documented explicit match-all form must be accepted by TheHive.
		rows := run(t, map[string]any{"_any": map[string]any{}})
		require.Len(t, rows, 4)
	})

	t.Run("omitted_filters_match_all", func(t *testing.T) {
		rows := run(t, nil)
		require.Len(t, rows, 4)
	})
}

// TestSearchInvalidFieldReturnsActionableError locks in the "self-correct, no
// inner retry" contract: when a filter references a field that does not exist,
// the tool returns an actionable error whose hint points the caller at the
// schema resources so it can correct the filter and call again.
func TestSearchInvalidFieldReturnsActionableError(t *testing.T) {
	hiveClient := testutils.SetupTestWithCleanup(t)
	createTestAlert(t, hiveClient, "Some Alert", 2, []string{"test"})

	mcpClient := testutils.GetMCPTestClient(t, unusedSamplingHandler(t), testutils.DummyElicitationAccept)

	result, err := mcpClient.CallTool(t.Context(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search-entities",
			Arguments: map[string]any{
				"entity-type": types.EntityTypeAlert,
				"filters":     map[string]any{"_eq": map[string]any{"_field": "thisFieldDoesNotExist", "_value": "x"}},
			},
		},
	})
	require.NoError(t, err)
	require.True(t, result.IsError, "searching on a non-existent field should return a tool error")

	text := resultText(t, result)
	require.Contains(t, text, "hive://schema/", "the error hint should point at the schema resources for self-correction")
}
