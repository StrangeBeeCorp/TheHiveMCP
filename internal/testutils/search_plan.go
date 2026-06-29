package testutils

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// SearchPlan mirrors the JSON search plan that the sampling handler returns to
// the search-entities tool. Zero values fall back to the defaults used across
// the search tests (desc sort by _createdAt, 10 results), so a test only needs
// to set the fields it actually varies.
type SearchPlan struct {
	// RawFilters is the raw_filters object as a JSON fragment (e.g.
	// `{"_any": ""}` or `{"_eq": {"_field": "severity", "_value": 3}}`).
	RawFilters string
	SortBy     string
	SortOrder  string
	NumResults int
	// KeptColumns defaults to ["_id", "title"] when nil.
	KeptColumns       []string
	ExtraData         []string
	AdditionalQueries []string
}

// SamplingPlanJSON renders plan as the JSON document the search-entities tool
// expects back from the sampling handler, applying the test defaults for any
// unset field. It fails the test if RawFilters is not valid JSON.
func SamplingPlanJSON(t *testing.T, plan SearchPlan) string {
	t.Helper()

	if plan.SortBy == "" {
		plan.SortBy = "_createdAt"
	}
	if plan.SortOrder == "" {
		plan.SortOrder = "desc"
	}
	if plan.NumResults == 0 {
		plan.NumResults = 10
	}
	if plan.KeptColumns == nil {
		plan.KeptColumns = []string{"_id", "title"}
	}
	if plan.ExtraData == nil {
		plan.ExtraData = []string{}
	}
	if plan.AdditionalQueries == nil {
		plan.AdditionalQueries = []string{}
	}

	rawFilters := json.RawMessage(plan.RawFilters)
	require.True(t, json.Valid(rawFilters), "raw_filters must be valid JSON: %s", plan.RawFilters)

	doc := map[string]any{
		"raw_filters":        rawFilters,
		"sort_by":            plan.SortBy,
		"sort_order":         plan.SortOrder,
		"num_results":        plan.NumResults,
		"kept_columns":       plan.KeptColumns,
		"extra_data":         plan.ExtraData,
		"additional_queries": plan.AdditionalQueries,
	}

	encoded, err := json.Marshal(doc)
	require.NoError(t, err)

	return string(encoded)
}

// SamplingPlanHandler is a convenience wrapper that renders plan via
// SamplingPlanJSON and returns a sampling handler that replies with it.
func SamplingPlanHandler(t *testing.T, plan SearchPlan) func(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	t.Helper()
	return SamplingHandlerCreateMessageFromStringResponse(SamplingPlanJSON(t, plan))
}
