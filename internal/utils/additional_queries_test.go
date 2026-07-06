package utils

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// TestFilterAdditionalQueryResultsSimilarityShapes pins the projection contract
// for the *Light similarity operations, whose output is FLAT and uniform across
// all four source↔hit pairs (CaseRenderer.lightSimilarCasesWrites /
// AlertRenderer.lightSimilarAlertsWrites): the entity fields and the
// match-context meta (similarObservableCount, observableCount) live side by side
// in one object, with no {"case"|"alert": {...}} wrapper.
//
// We assert the default entity fields and the stats both survive projection for
// every pair, and that linkedWith — dropped from the surfaced meta when we
// migrated to Light — does NOT leak through even when present in the input. Each
// pair resolves its real descriptor from the registry, so the projection is
// driven by the same QueryDescriptor.MetaFields the production path uses.
func TestFilterAdditionalQueryResultsSimilarityShapes(t *testing.T) {
	cases := []struct {
		name       string
		parentType string
		queryName  string
		idField    string
	}{
		{"alert→similarCases", types.EntityTypeAlert, querySimilarCases, "~1"},
		{"case→similarCases", types.EntityTypeCase, querySimilarCases, "~2"},
		{"case→similarAlerts", types.EntityTypeCase, querySimilarAlerts, "~3"},
		{"alert→similarAlerts", types.EntityTypeAlert, querySimilarAlerts, "~4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			descriptor, ok := queryRegistry[tc.parentType][tc.queryName]
			require.True(t, ok, "query %q must be registered for %q", tc.queryName, tc.parentType)
			require.True(t, descriptor.ResultsAreIndependent,
				"similarity queries must be declared independent so their hits are re-scoped")

			in := []map[string]any{
				{
					fieldID:                     tc.idField,
					fieldTitle:                  "Light hit",
					fieldSeverity:               2,
					fieldSimilarObservableCount: float64(3),
					fieldObservableCount:        float64(5),
					"linksCount":                float64(3),
					// linkedWith is no longer surfaced; include it to prove it is dropped.
					"linkedWith": map[string]any{"ip": []any{"1.1.1.1"}},
				},
			}
			out, err := filterAdditionalQueryResults(in, descriptor)
			require.NoError(t, err)
			require.Len(t, out, 1)
			require.Equal(t, tc.idField, out[0][fieldID])
			require.Equal(t, "Light hit", out[0][fieldTitle])
			require.InEpsilon(t, float64(3), out[0][fieldSimilarObservableCount], 1e-9,
				"flat Light similarity results must surface similarObservableCount")
			require.InEpsilon(t, float64(5), out[0][fieldObservableCount], 1e-9)
			require.NotContains(t, out[0], "linkedWith",
				"linkedWith was dropped from the surfaced meta on the Light migration")
		})
	}
}

// TestEveryRegisteredQueryDeclaresScopeIntent is the structural fail-closed
// guard. Every descriptor in the registry must declare a fetch Func and a result
// EntityType — a query added with a zero-value descriptor (e.g. forgetting
// EntityType, or whose results are independent but ResultsAreIndependent was left
// false) would slip past projection or scope re-checking. Asserting the shape of
// every entry catches that at test time rather than as a runtime leak.
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

// TestFilterAdditionalQueryResultsNonSimilarityDropsMeta pins the
// non-similarity branch: a normal additional query (e.g. tasks) projects only
// the default entity fields and never lifts match-context meta fields, even
// when an input map happens to carry them. The tasks descriptor declares no
// MetaFields and ResultsAreIndependent=false, so includeMeta is false.
func TestFilterAdditionalQueryResultsNonSimilarityDropsMeta(t *testing.T) {
	descriptor, ok := queryRegistry[types.EntityTypeCase]["tasks"]
	require.True(t, ok, "tasks must be registered for case")
	require.False(t, descriptor.ResultsAreIndependent,
		"tasks results are children of an already-scoped parent, not independent")
	require.Empty(t, descriptor.MetaFields, "tasks declares no match-context meta")

	in := []map[string]any{
		{
			fieldID:    "~10",
			fieldTitle: "Investigate",
			// Meta fields must NOT survive for a non-similarity query.
			fieldSimilarObservableCount: float64(3),
			fieldObservableCount:        float64(5),
		},
	}
	out, err := filterAdditionalQueryResults(in, descriptor)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "~10", out[0][fieldID])
	require.Equal(t, "Investigate", out[0][fieldTitle])
	require.NotContains(t, out[0], fieldSimilarObservableCount,
		"non-similarity queries must not surface similarity meta fields")
	require.NotContains(t, out[0], fieldObservableCount,
		"non-similarity queries must not surface similarity meta fields")
}

// TestFilterSimilarityHitsByScopeLogsUnresolvableDrops proves the fail-closed
// drop path in filterSimilarityHitsByScope is no longer silent: a hit with no
// resolvable string _id is dropped AND emits a Debug log (per-hit line +
// aggregate droppedCount/totalHits), so a future _id shape regression in the
// *Light output is diagnosable instead of silently producing empty similarity
// results. In-scope hits are kept; legitimate out-of-scope drops and all-in-scope
// input stay quiet (no regression signal to surface).
func TestFilterSimilarityHitsByScopeLogsUnresolvableDrops(t *testing.T) {
	t.Run("unresolvable _id is dropped and logged", func(t *testing.T) {
		buf := captureDebugLogs(t)

		results := []map[string]any{
			{fieldID: "~1", fieldTitle: "valid one"},
			{fieldID: float64(123), fieldTitle: "numeric id"}, // not a string → no resolvable _id
			{fieldID: "~2", fieldTitle: "valid two"},
		}
		inScope := map[string]bool{"~1": true, "~2": true}

		kept := filterSimilarityHitsByScope(context.Background(), querySimilarCases, "case", results, inScope)

		require.Len(t, kept, 2, "only resolvable, in-scope hits are kept")
		require.Equal(t, "~1", kept[0][fieldID])
		require.Equal(t, "~2", kept[1][fieldID])

		logs := buf.String()
		require.Contains(t, logs, "Dropping similarity hit with no resolvable _id",
			"the per-hit drop must be logged so the regression is diagnosable")
		require.Contains(t, logs, "query=similarCases")
		require.Contains(t, logs, "targetType=case")
		require.Contains(t, logs, "droppedCount=1", "the aggregate line must report how many were dropped")
		require.Contains(t, logs, "totalHits=3")
		// hitKeys carries the malformed hit's keys (not values) to reveal the shape.
		require.Contains(t, logs, fieldTitle)
	})

	t.Run("out-of-scope drop is silent", func(t *testing.T) {
		buf := captureDebugLogs(t)

		results := []map[string]any{
			{fieldID: "~1", fieldTitle: "in scope"},
			{fieldID: "~2", fieldTitle: "out of scope"},
		}
		inScope := map[string]bool{"~1": true} // ~2 resolvable but not in scope

		kept := filterSimilarityHitsByScope(context.Background(), querySimilarAlerts, "alert", results, inScope)

		require.Len(t, kept, 1)
		require.Equal(t, "~1", kept[0][fieldID])
		require.Empty(t, buf.String(),
			"an expected out-of-scope drop carries no regression signal and must not be logged")
	})

	t.Run("all in-scope input logs nothing", func(t *testing.T) {
		buf := captureDebugLogs(t)

		results := []map[string]any{
			{fieldID: "~1", fieldTitle: "valid one"},
			{fieldID: "~2", fieldTitle: "valid two"},
		}
		inScope := map[string]bool{"~1": true, "~2": true}

		kept := filterSimilarityHitsByScope(context.Background(), querySimilarAlerts, "alert", results, inScope)

		require.Len(t, kept, 2)
		require.Empty(t, buf.String(), "no hit was dropped, so nothing must be logged")
	})
}

// captureDebugLogs redirects the default slog logger to an in-memory buffer at
// Debug level for the duration of the test, restoring the previous logger after.
func captureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	prev := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	return buf
}
