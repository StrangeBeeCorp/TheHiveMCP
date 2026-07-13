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
