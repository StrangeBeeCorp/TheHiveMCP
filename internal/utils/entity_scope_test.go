package utils

import (
	"context"
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
