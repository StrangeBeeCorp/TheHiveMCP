package resources

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestValidateResponderParams(t *testing.T) {
	tests := []struct {
		name       string
		entityType string
		entityID   string
		wantErr    string
	}{
		{name: "valid case entity", entityType: "case", entityID: "~123456"},
		{name: "valid alert entity", entityType: "alert", entityID: "~409640"},
		{name: "valid observable entity", entityType: "observable", entityID: "~123456"},
		{name: "valid case_artifact entity", entityType: "case_artifact", entityID: "~123456"},
		{name: "valid long id", entityType: "case", entityID: "~40968404128"},

		{name: "entity type not in allowlist", entityType: "user", entityID: "~123456", wantErr: "invalid entityType"},
		{name: "entity type with traversal", entityType: "../admin", entityID: "~123456", wantErr: "invalid entityType"},
		{name: "entity type with slash", entityType: "case/extra", entityID: "~123456", wantErr: "invalid entityType"},

		{name: "entity id missing tilde prefix", entityType: "case", entityID: "123456", wantErr: "invalid entityId"},
		{name: "entity id with letters", entityType: "task", entityID: "abc123", wantErr: "invalid entityId"},
		{name: "entity id with dash and underscore", entityType: "case", entityID: "id-with_token", wantErr: "invalid entityId"},
		{name: "entity id with relative traversal", entityType: "case", entityID: "../../../api/v1/user", wantErr: "invalid entityId"},
		{name: "entity id with slash", entityType: "case", entityID: "~123/456", wantErr: "invalid entityId"},
		{name: "entity id with dot-dot", entityType: "case", entityID: "..", wantErr: "invalid entityId"},
		{name: "entity id with url-encoded traversal", entityType: "case", entityID: "%2e%2e%2f", wantErr: "invalid entityId"},
		{name: "entity id with fragment", entityType: "case", entityID: "~123#frag", wantErr: "invalid entityId"},
		{name: "entity id with query", entityType: "case", entityID: "~123?x=1", wantErr: "invalid entityId"},
		{name: "entity id with space", entityType: "case", entityID: "~123 456", wantErr: "invalid entityId"},
		{name: "entity id with backslash", entityType: "case", entityID: `..\..\admin`, wantErr: "invalid entityId"},
		{name: "entity id tilde only", entityType: "case", entityID: "~", wantErr: "invalid entityId"},
		{name: "empty entity id", entityType: "case", entityID: "", wantErr: "invalid entityId"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResponderParams(tt.entityType, tt.entityID)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

// Validation must run before the client is resolved, so a rejected value never
// reaches a Cortex request.
func TestGetAvailableRespondersValidatesBeforeAnyCall(t *testing.T) {
	makeRequest := func(entityType, entityID string) mcp.ReadResourceRequest {
		return mcp.ReadResourceRequest{
			Params: mcp.ReadResourceParams{
				URI: "hive://metadata/automation/responders",
				Arguments: map[string]any{
					"entityType": entityType,
					"entityId":   entityID,
				},
			},
		}
	}

	// Traversal payloads are rejected with a validation error, not a missing
	// client error: nothing past validation executed.
	for _, payload := range []string{"../../../api/v1/user", "~123/456", "%2e%2e", "~123#frag"} {
		_, err := GetAvailableResponders(context.Background(), makeRequest("case", payload))
		require.ErrorContains(t, err, "invalid entityId")
	}
	_, err := GetAvailableResponders(context.Background(), makeRequest("../case", "~123456"))
	require.ErrorContains(t, err, "invalid entityType")

	// Valid parameters pass validation and proceed to the client lookup
	// (which fails here because the bare context carries no client).
	_, err = GetAvailableResponders(context.Background(), makeRequest("case", "~123456"))
	require.ErrorContains(t, err, "failed to get TheHive client")
}
