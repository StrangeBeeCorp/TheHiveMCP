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

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/stretchr/testify/require"
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
		{types.EntityTypeCase, "getCase"},
		{types.EntityTypeAlert, "getAlert"},
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

// Without configured filters every requested ID is in scope and no TheHive
// call is made (the bare context carries no client, so a call would fail).
func TestGetEntityIDsInScopeWithoutFilters(t *testing.T) {
	inScope, err := GetEntityIDsInScope(context.Background(), types.EntityTypeCase, []string{"~1", "~2"}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"~1": true, "~2": true}, inScope)

	ok, err := IsEntityInScope(context.Background(), types.EntityTypeCase, "~1", map[string]interface{}{})
	require.NoError(t, err)
	require.True(t, ok)
}

// With filters configured but no IDs to check, nothing is queried.
func TestGetEntityIDsInScopeWithoutIDs(t *testing.T) {
	filters := map[string]interface{}{"_lte": map[string]interface{}{"_field": "tlp", "_value": 2}}
	inScope, err := GetEntityIDsInScope(context.Background(), types.EntityTypeCase, nil, filters)
	require.NoError(t, err)
	require.Empty(t, inScope)
}

// GetScopedEntityIDsBatch shares its short-circuit guards with
// GetEntityIDsInScope (both delegate to scopedEntityIDsBatch, differing only in
// concurrency). These mirror the GetEntityIDsInScope guard tests for the batch
// entry point, proving the shared no-query paths behave identically. Like the
// serial tests they pass a bare context with no TheHive client, so reaching a
// query would fail — confirming neither guard issues one.
func TestGetScopedEntityIDsBatchWithoutFilters(t *testing.T) {
	inScope, err := GetScopedEntityIDsBatch(context.Background(), types.EntityTypeCase, []string{"~1", "~2"}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"~1": true, "~2": true}, inScope)
}

func TestGetScopedEntityIDsBatchWithoutIDs(t *testing.T) {
	filters := map[string]interface{}{"_lte": map[string]interface{}{"_field": "tlp", "_value": 2}}
	inScope, err := GetScopedEntityIDsBatch(context.Background(), types.EntityTypeCase, nil, filters)
	require.NoError(t, err)
	require.Empty(t, inScope)
}

// TestGetScopedEntityIDsBatchStopsDispatchingOnCancel proves the fan-out honours
// context cancellation: when the caller's ctx is cancelled mid-batch, the
// dispatch loop must stop issuing the remaining per-ID scope checks (rather than
// firing every one for a result that will be discarded) and the call must report
// an error instead of a silently-truncated partial map.
//
// It runs entirely against an httptest fake (no Docker), so it executes under
// `make test`. The handler cancels the caller's ctx the moment the first scope
// query lands, then counts how many further queries arrive. With 50 IDs and a
// concurrency cap of 8, an honest implementation issues far fewer than 50 before
// the loop notices the cancellation and breaks.
func TestGetScopedEntityIDsBatchStopsDispatchingOnCancel(t *testing.T) {
	const total = 50

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var queries int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed struct {
			Query []map[string]interface{} `json:"query"`
		}
		_ = json.Unmarshal(body, &parsed)

		names := operationNames(parsed.Query)
		if len(names) > 0 && names[0] == "getCase" && slices.Contains(names, "filter") {
			// Cancel on the first query, then keep counting so the test can assert
			// the loop stopped dispatching the rest.
			if atomic.AddInt64(&queries, 1) == 1 {
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
	permFilters := map[string]interface{}{
		"_lte": map[string]interface{}{"_field": "tlp", "_value": 2},
	}

	inScope, err := GetScopedEntityIDsBatch(qctx, types.EntityTypeCase, entityIDs, permFilters)
	require.Error(t, err, "a cancelled batch must report an error, not a truncated partial map")
	require.Nil(t, inScope, "the partial map must be discarded on cancellation")

	got := atomic.LoadInt64(&queries)
	require.Less(t, got, int64(total),
		"dispatch must stop after cancellation, not fire all %d checks (got %d)", total, got)
}
