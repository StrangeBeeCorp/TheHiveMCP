package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

func TestListOperationName(t *testing.T) {
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
	inScope, err := GetEntityIDsInScope(context.Background(), types.EntityTypeCase, []string{"~1", "~2"}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"~1": true, "~2": true}, inScope)

	ok, err := IsEntityInScope(context.Background(), types.EntityTypeCase, "~1", map[string]any{})
	require.NoError(t, err)
	require.True(t, ok)
}

// With filters configured but no IDs to check, nothing is queried.
func TestGetEntityIDsInScopeWithoutIDs(t *testing.T) {
	filters := map[string]any{opLTE: map[string]any{fieldField: fieldTLP, fieldValue: 2}}
	inScope, err := GetEntityIDsInScope(context.Background(), types.EntityTypeCase, nil, filters)
	require.NoError(t, err)
	require.Empty(t, inScope)
}

// Batch entry point, same bare-context guard as TestGetEntityIDsInScopeWithoutFilters.
func TestGetScopedEntityIDsBatchWithoutFilters(t *testing.T) {
	inScope, err := GetScopedEntityIDsBatch(context.Background(), types.EntityTypeCase, []string{"~1", "~2"}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"~1": true, "~2": true}, inScope)
}

func TestGetScopedEntityIDsBatchWithoutIDs(t *testing.T) {
	filters := map[string]any{opLTE: map[string]any{fieldField: fieldTLP, fieldValue: 2}}
	inScope, err := GetScopedEntityIDsBatch(context.Background(), types.EntityTypeCase, nil, filters)
	require.NoError(t, err)
	require.Empty(t, inScope)
}

// On mid-batch ctx cancellation the dispatch loop must stop issuing remaining
// per-ID checks and report an error, not a silently-truncated partial map. The
// handler cancels on the first query; with 50 IDs and a concurrency cap of 8, an
// honest implementation issues far fewer than 50 before noticing the cancellation.
func TestGetScopedEntityIDsBatchStopsDispatchingOnCancel(t *testing.T) {
	const total = 50

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var queries atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		var parsed struct {
			Query []map[string]any `json:"query"`
		}

		_ = json.Unmarshal(body, &parsed)

		names := operationNames(parsed.Query)
		if len(names) > 0 && names[0] == opGetCase && slices.Contains(names, "filter") {
			// Cancel on the first query but keep counting, so the test can assert
			// the loop stopped dispatching the rest.
			if queries.Add(1) == 1 {
				cancel()
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))

			return
		}

		http.Error(w, "unexpected query", http.StatusInternalServerError)
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
	require.Error(t, err, "a cancelled batch must report an error, not a truncated partial map")
	require.Nil(t, inScope, "the partial map must be discarded on cancellation")

	got := queries.Load()
	require.Less(t, got, int64(total),
		"dispatch must stop after cancellation, not fire all %d checks (got %d)", total, got)
}
