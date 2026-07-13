package utils

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestListOperationName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		entityType string
		want       string
	}{
		{types.EntityTypeCase, "listCase"},
		{types.EntityTypeAlert, "listAlert"},
		{types.EntityTypeTask, "listTask"},
		{types.EntityTypeObservable, "listObservable"},
		{types.EntityTypeProcedure, "listProcedure"},
		{types.EntityTypePage, "listPage"},
		{types.EntityTypeCaseTemplate, "listCaseTemplate"},
		{"", ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, ListOperationName(tt.entityType), "entityType %q", tt.entityType)
	}
}

func TestGetOperationName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		entityType string
		want       string
	}{
		{types.EntityTypeCase, opGetCase},
		{types.EntityTypeAlert, opGetAlert},
		{types.EntityTypeTask, "getTask"},
		{types.EntityTypeObservable, "getObservable"},
		{types.EntityTypeProcedure, "getProcedure"},
		{types.EntityTypePage, "getPage"},
		{types.EntityTypeCaseTemplate, "getCaseTemplate"},
		{"", ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, getOperationName(tt.entityType), "entityType %q", tt.entityType)
	}
}

// No filters: every requested ID is in scope with no TheHive call (the bare
// context carries no client, so a call would fail).
func TestGetEntityIDsInScopeWithoutFilters(t *testing.T) {
	t.Parallel()

	inScope, err := GetEntityIDsInScope(context.Background(), types.EntityTypeCase, []string{"~1", "~2"}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"~1": true, "~2": true}, inScope)

	ok, err := IsEntityInScope(context.Background(), types.EntityTypeCase, "~1", map[string]any{})
	require.NoError(t, err)
	require.True(t, ok)
}

// With filters configured but no IDs to check, nothing is queried.
func TestGetEntityIDsInScopeWithoutIDs(t *testing.T) {
	t.Parallel()

	filters := map[string]any{opLTE: map[string]any{fieldField: fieldTLP, fieldValue: 2}}
	inScope, err := GetEntityIDsInScope(context.Background(), types.EntityTypeCase, nil, filters)
	require.NoError(t, err)
	require.Empty(t, inScope)
}

// Batch entry point, same bare-context guard as TestGetEntityIDsInScopeWithoutFilters.
func TestGetScopedEntityIDsBatchWithoutFilters(t *testing.T) {
	t.Parallel()

	inScope, err := GetScopedEntityIDsBatch(context.Background(), types.EntityTypeCase, []string{"~1", "~2"}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"~1": true, "~2": true}, inScope)
}

func TestGetScopedEntityIDsBatchWithoutIDs(t *testing.T) {
	t.Parallel()

	filters := map[string]any{opLTE: map[string]any{fieldField: fieldTLP, fieldValue: 2}}
	inScope, err := GetScopedEntityIDsBatch(context.Background(), types.EntityTypeCase, nil, filters)
	require.NoError(t, err)
	require.Empty(t, inScope)
}

// The batch is fail-closed: a cancelled context must surface as an error with no
// map, never a truncated partial map treated as complete. The single list query
// runs against a context cancelled before dispatch, so the request fails outright.
func TestGetScopedEntityIDsBatchFailsClosedOnCancel(t *testing.T) {
	t.Parallel()

	const total = 50

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	client := newFakeHiveClient(t, srv)
	qctx := context.WithValue(ctx, types.HiveClientCtxKey, client)

	entityIDs := make([]string, total)
	for i := range entityIDs {
		entityIDs[i] = fmt.Sprintf("~%d", i)
	}

	permFilters := map[string]any{
		opLTE: map[string]any{fieldField: fieldTLP, fieldValue: 2},
	}

	inScope, err := GetScopedEntityIDsBatch(qctx, types.EntityTypeCase, entityIDs, permFilters)
	require.Error(t, err, "a cancelled batch must report an error, not a partial map")
	require.Nil(t, inScope, "the partial map must be discarded on cancellation")
}

// A batch larger than one chunk boundary must be split into ceil(N/chunk) list
// queries and the matched sets unioned, so it can never exceed a server-side
// _in-clause / page-row cap and silently come back truncated as a false "out of
// scope" (DL-5764, mass false denial). The unioned in-scope verdict must match
// the per-ID oracle exactly, and the number of round-trips must equal the number
// of chunks — proving the split actually happened rather than one oversized query.
func TestGetScopedEntityIDsBatchChunksLargeIDSet(t *testing.T) {
	t.Parallel()

	// Straddle a chunk boundary: 2 full chunks + a partial third.
	total := 2*scopeBatchChunkSize + 7

	entityIDs := make([]string, total)
	scopeIDs := make(map[string]bool, total)
	oracle := make(map[string]bool, total)

	for i := range entityIDs {
		id := fmt.Sprintf("~%d", i)
		entityIDs[i] = id
		// Every third id is out of scope, so the union must exclude some ids per chunk.
		inScope := i%3 != 0
		scopeIDs[id] = inScope
		oracle[id] = inScope
	}

	fake := &fakeHiveServer{
		scopeIDs:        scopeIDs,
		scopeCheckedIDs: map[string]int{},
	}
	ctx := startFakeHive(t, fake)

	inScope, err := GetScopedEntityIDsBatch(ctx, types.EntityTypeCase, entityIDs, tlpLTE2Filters())
	require.NoError(t, err)
	require.Equal(t, oracle, inScope,
		"the chunked union must match the per-ID oracle: every in-scope id present-and-true, every out-of-scope id present-and-false")

	wantChunks := (total + scopeBatchChunkSize - 1) / scopeBatchChunkSize
	require.Equal(t, int64(wantChunks), fake.scopeQueries.Load(),
		"a batch of %d ids must split into %d chunk queries, not one oversized _in list", total, wantChunks)

	// No chunk query may request more than scopeBatchChunkSize ids.
	fake.mu.Lock()
	defer fake.mu.Unlock()

	require.LessOrEqual(t, fake.maxScopeQuerySize, scopeBatchChunkSize,
		"no single chunk query may exceed the chunk size")

	for id, n := range fake.scopeCheckedIDs {
		require.Equal(t, 1, n, "id %s must be checked exactly once across all chunks", id)
	}
}

// The scope builders normalize + date-translate permFilters in place via
// mutating recursive transforms. A shallow copy would leave nested maps/slices
// aliased with the caller's original, so those transforms would rewrite the
// caller's permFilters (e.g. turn a date string into epoch millis). Deep-copying
// must keep the input untouched.
func TestScopeBuildersDoNotMutateCallerPermFilters(t *testing.T) {
	t.Parallel()

	const dateStr = "2024-01-01T00:00:00"

	newPermFilters := func() map[string]any {
		return map[string]any{
			opAnd: []any{
				map[string]any{opGTE: map[string]any{fieldField: "createdAt", fieldValue: dateStr}},
				map[string]any{`"` + opLTE + `"`: map[string]any{fieldField: fieldTLP, fieldValue: 2}},
			},
		}
	}

	for _, tc := range []struct {
		name  string
		build func(map[string]any)
	}{
		{"scopeFilterOperation", func(pf map[string]any) { scopeFilterOperation(pf) }},
		{"scopeInFilterOperation", func(pf map[string]any) { scopeInFilterOperation(pf, []string{"~1", "~2"}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			permFilters := newPermFilters()

			tc.build(permFilters)

			require.Equal(t, newPermFilters(), permFilters,
				"scope builder must not mutate the caller's permFilters (nested date string / malformed key)")
		})
	}
}
