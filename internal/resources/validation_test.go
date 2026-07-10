package resources

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

const (
	testEntityTypeCase       = "case"
	testErrInvalidEntityType = "invalid entityType"
	testErrInvalidEntityID   = "invalid entityId"
	testEntityID             = "~123456"
)

func TestValidateResponderParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entityType string
		entityID   string
		wantErr    string
	}{
		{name: "valid case entity", entityType: testEntityTypeCase, entityID: testEntityID},
		{name: "valid alert entity", entityType: "alert", entityID: "~409640"},
		{name: "valid observable entity", entityType: "observable", entityID: testEntityID},
		{name: "valid case_artifact entity", entityType: "case_artifact", entityID: testEntityID},
		{name: "valid long id", entityType: testEntityTypeCase, entityID: "~40968404128"},

		{name: "entity type not in allowlist", entityType: "user", entityID: testEntityID, wantErr: testErrInvalidEntityType},
		{name: "entity type with traversal", entityType: "../admin", entityID: testEntityID, wantErr: testErrInvalidEntityType},
		{name: "entity type with slash", entityType: "case/extra", entityID: testEntityID, wantErr: testErrInvalidEntityType},

		{name: "entity id missing tilde prefix", entityType: testEntityTypeCase, entityID: "123456", wantErr: testErrInvalidEntityID},
		{name: "entity id with letters", entityType: "task", entityID: "abc123", wantErr: testErrInvalidEntityID},
		{name: "entity id with dash and underscore", entityType: testEntityTypeCase, entityID: "id-with_token", wantErr: testErrInvalidEntityID},
		{name: "entity id with relative traversal", entityType: testEntityTypeCase, entityID: "../../../api/v1/user", wantErr: testErrInvalidEntityID},
		{name: "entity id with slash", entityType: testEntityTypeCase, entityID: "~123/456", wantErr: testErrInvalidEntityID},
		{name: "entity id with dot-dot", entityType: testEntityTypeCase, entityID: "..", wantErr: testErrInvalidEntityID},
		{name: "entity id with url-encoded traversal", entityType: testEntityTypeCase, entityID: "%2e%2e%2f", wantErr: testErrInvalidEntityID},
		{name: "entity id with fragment", entityType: testEntityTypeCase, entityID: "~123#frag", wantErr: testErrInvalidEntityID},
		{name: "entity id with query", entityType: testEntityTypeCase, entityID: "~123?x=1", wantErr: testErrInvalidEntityID},
		{name: "entity id with space", entityType: testEntityTypeCase, entityID: "~123 456", wantErr: testErrInvalidEntityID},
		{name: "entity id with backslash", entityType: testEntityTypeCase, entityID: `..\..\admin`, wantErr: testErrInvalidEntityID},
		{name: "entity id tilde only", entityType: testEntityTypeCase, entityID: "~", wantErr: testErrInvalidEntityID},
		{name: "empty entity id", entityType: testEntityTypeCase, entityID: "", wantErr: testErrInvalidEntityID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
	t.Parallel()

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
		_, err := GetAvailableResponders(context.Background(), makeRequest(testEntityTypeCase, payload))
		require.ErrorContains(t, err, testErrInvalidEntityID)
	}

	_, err := GetAvailableResponders(context.Background(), makeRequest("../case", testEntityID))
	require.ErrorContains(t, err, testErrInvalidEntityType)

	// Valid parameters pass validation and proceed to the client lookup
	// (which fails here because the bare context carries no client).
	_, err = GetAvailableResponders(context.Background(), makeRequest(testEntityTypeCase, testEntityID))
	require.ErrorContains(t, err, "failed to get TheHive client")
}
