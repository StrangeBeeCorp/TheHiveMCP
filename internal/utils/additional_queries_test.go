package utils

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/stretchr/testify/require"
)

// Flat *Light similarity output: entity fields + meta side by side, no
// {"case"|"alert"} wrapper. Meta stats survive projection for all four
// source↔hit pairs; dropped linkedWith does not leak through.
func TestFilterAdditionalQueryResultsSimilarityShapes(t *testing.T) {
	cases := []struct {
		name       string
		parentType string
		queryName  string
		idField    string
	}{
		{"alert→similarCases", types.EntityTypeAlert, "similarCases", "~1"},
		{"case→similarCases", types.EntityTypeCase, "similarCases", "~2"},
		{"case→similarAlerts", types.EntityTypeCase, "similarAlerts", "~3"},
		{"alert→similarAlerts", types.EntityTypeAlert, "similarAlerts", "~4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			descriptor, ok := queryRegistry[tc.parentType][tc.queryName]
			require.True(t, ok, "query %q must be registered for %q", tc.queryName, tc.parentType)
			require.True(t, descriptor.ResultsAreIndependent,
				"similarity queries must be declared independent so their hits are re-scoped")

			in := []map[string]interface{}{
				{
					"_id":                    tc.idField,
					"title":                  "Light hit",
					"severity":               2,
					"similarObservableCount": float64(3),
					"observableCount":        float64(5),
					"linksCount":             float64(3),
					// linkedWith is no longer surfaced; include it to prove it is dropped.
					"linkedWith": map[string]interface{}{"ip": []interface{}{"1.1.1.1"}},
				},
			}
			out, err := filterAdditionalQueryResults(in, descriptor)
			require.NoError(t, err)
			require.Len(t, out, 1)
			require.Equal(t, tc.idField, out[0]["_id"])
			require.Equal(t, "Light hit", out[0]["title"])
			require.Equal(t, float64(3), out[0]["similarObservableCount"],
				"flat Light similarity results must surface similarObservableCount")
			require.Equal(t, float64(5), out[0]["observableCount"])
			require.NotContains(t, out[0], "linkedWith",
				"linkedWith was dropped from the surfaced meta on the Light migration")
		})
	}
}

// Fail-closed guard: a zero-value descriptor (missing Func/EntityType) would slip
// past projection or scope re-checking. Catch it at test time, not as a runtime leak.
func TestEveryRegisteredQueryDeclaresScopeIntent(t *testing.T) {
	for entityType, config := range queryRegistry {
		for queryName, descriptor := range config {
			require.NotNilf(t, descriptor.Func,
				"%s/%s: descriptor must declare a fetch Func", entityType, queryName)
			require.NotEmptyf(t, descriptor.EntityType,
				"%s/%s: descriptor must declare a result EntityType", entityType, queryName)
			require.Containsf(t, types.DefaultFields, descriptor.EntityType,
				"%s/%s: result EntityType %q must have DefaultFields to project",
				entityType, queryName, descriptor.EntityType)
		}
	}
}

// Tasks declares no MetaFields and ResultsAreIndependent=false, so includeMeta is
// false: meta fields in the input must not survive projection.
func TestFilterAdditionalQueryResultsNonSimilarityDropsMeta(t *testing.T) {
	descriptor, ok := queryRegistry[types.EntityTypeCase]["tasks"]
	require.True(t, ok, "tasks must be registered for case")
	require.False(t, descriptor.ResultsAreIndependent,
		"tasks results are children of an already-scoped parent, not independent")
	require.Empty(t, descriptor.MetaFields, "tasks declares no match-context meta")

	in := []map[string]interface{}{
		{
			"_id":                    "~10",
			"title":                  "Investigate",
			"similarObservableCount": float64(3),
			"observableCount":        float64(5),
		},
	}
	out, err := filterAdditionalQueryResults(in, descriptor)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "~10", out[0]["_id"])
	require.Equal(t, "Investigate", out[0]["title"])
	require.NotContains(t, out[0], "similarObservableCount",
		"non-similarity queries must not surface similarity meta fields")
	require.NotContains(t, out[0], "observableCount",
		"non-similarity queries must not surface similarity meta fields")
}

// An unresolvable-_id drop is Debug-logged so a future *Light _id shape regression
// is diagnosable rather than silently emptying results; expected out-of-scope and
// all-in-scope drops stay quiet, carrying no regression signal.
func TestFilterSimilarityHitsByScopeLogsUnresolvableDrops(t *testing.T) {
	t.Run("unresolvable _id is dropped and logged", func(t *testing.T) {
		buf := captureDebugLogs(t)

		results := []map[string]interface{}{
			{"_id": "~1", "title": "valid one"},
			{"_id": float64(123), "title": "numeric id"}, // not a string → no resolvable _id
			{"_id": "~2", "title": "valid two"},
		}
		inScope := map[string]bool{"~1": true, "~2": true}

		kept := filterSimilarityHitsByScope(context.Background(), "similarCases", "case", results, inScope)

		require.Len(t, kept, 2, "only resolvable, in-scope hits are kept")
		require.Equal(t, "~1", kept[0]["_id"])
		require.Equal(t, "~2", kept[1]["_id"])

		logs := buf.String()
		require.Contains(t, logs, "Dropping similarity hit with no resolvable _id",
			"the per-hit drop must be logged so the regression is diagnosable")
		require.Contains(t, logs, "query=similarCases")
		require.Contains(t, logs, "targetType=case")
		require.Contains(t, logs, "droppedCount=1", "the aggregate line must report how many were dropped")
		require.Contains(t, logs, "totalHits=3")
		// hitKeys carries the malformed hit's keys (not values) to reveal the shape.
		require.Contains(t, logs, "title")
	})

	t.Run("out-of-scope drop is silent", func(t *testing.T) {
		buf := captureDebugLogs(t)

		results := []map[string]interface{}{
			{"_id": "~1", "title": "in scope"},
			{"_id": "~2", "title": "out of scope"},
		}
		inScope := map[string]bool{"~1": true} // ~2 resolvable but not in scope

		kept := filterSimilarityHitsByScope(context.Background(), "similarAlerts", "alert", results, inScope)

		require.Len(t, kept, 1)
		require.Equal(t, "~1", kept[0]["_id"])
		require.Empty(t, buf.String(),
			"an expected out-of-scope drop carries no regression signal and must not be logged")
	})

	t.Run("all in-scope input logs nothing", func(t *testing.T) {
		buf := captureDebugLogs(t)

		results := []map[string]interface{}{
			{"_id": "~1", "title": "valid one"},
			{"_id": "~2", "title": "valid two"},
		}
		inScope := map[string]bool{"~1": true, "~2": true}

		kept := filterSimilarityHitsByScope(context.Background(), "similarAlerts", "alert", results, inScope)

		require.Len(t, kept, 2)
		require.Empty(t, buf.String(), "no hit was dropped, so nothing must be logged")
	})
}

func captureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}
