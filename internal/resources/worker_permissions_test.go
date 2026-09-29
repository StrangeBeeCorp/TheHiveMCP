package resources_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// Regression: the catalog kept a responder blocked by name, because its ID
// was not blocked, and pointed the model at a worker the rules exclude.
func TestResponderCatalogHidesAResponderBlockedByName(t *testing.T) {
	t.Parallel()

	deleteCase := thehive.OutputWorker{Id: "d3e4f5", Name: "DeleteCase_1_0", Version: "1.0", Description: "Deletes the case"}
	mailer := thehive.OutputWorker{Id: "a6b7c8", Name: "Mailer_1_0", Version: "1.0", Description: "Sends an email"}
	cortex := &testutils.FakeCortex{Responders: []thehive.OutputWorker{deleteCase, mailer}}

	perms, err := permissions.ParseYAML([]byte(`version: "1.0"
permissions:
  responders:
    mode: "block_list"
    blocked: ["DeleteCase_1_0"]
`))
	require.NoError(t, err)

	ctx := utils.AddPermissionsToContext(context.Background(), perms)
	ctx = context.WithValue(ctx, types.HiveClientCtxKey, cortex.Client(t))

	contents, err := resources.GetAvailableResponders(ctx, mcp.ReadResourceRequest{Params: mcp.ReadResourceParams{
		Arguments: map[string]any{"entityType": types.EntityTypeCase, "entityId": "~1"},
	}})
	require.NoError(t, err)
	require.Len(t, contents, 1)

	text, ok := contents[0].(mcp.TextResourceContents)
	require.True(t, ok)

	var page struct {
		BlockedByPolicy int                    `json:"blockedByPolicy"`
		Workers         []thehive.OutputWorker `json:"workers"`
	}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &page))

	assert.Equal(t, 1, page.BlockedByPolicy)
	require.Len(t, page.Workers, 1)
	assert.Equal(t, mailer.Id, page.Workers[0].Id)
}
