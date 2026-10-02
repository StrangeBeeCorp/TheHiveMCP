package surface_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/surface"
)

// A prompt the server advertises is part of what the model reads: it has its snapshot file, its
// tokens, and a budget that starts at none.
func TestAPromptIsSnapshottedAndBudgeted(t *testing.T) {
	t.Parallel()

	srv := server.NewMCPServer("t", "1", server.WithPromptCapabilities(false))
	srv.AddPrompt(mcp.NewPrompt("triage", mcp.WithPromptDescription("Triage an alert step by step")),
		func(context.Context, mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{}, nil
		})

	snap, err := surface.TakeFrom(t.Context(), srv)
	require.NoError(t, err)
	require.Len(t, snap.Prompts, 1)
	assert.Equal(t, "triage", snap.Prompts[0].Name)

	files, err := snap.Files()
	require.NoError(t, err)
	assert.Contains(t, string(files["prompts/triage.json"]), "Triage an alert step by step")

	sum := snap.Summary()
	assert.Equal(t, snap.Prompts[0].Tokens, sum.PromptsTotal)
	assert.Positive(t, sum.PromptsTotal)
	assert.Contains(t, surface.Budgets{ToolsTotal: 1, Tool: 1, ResourcesTotal: 1}.Over(sum),
		"the prompts cost "+strconv.Itoa(sum.PromptsTotal)+" tokens together, over the budget of 0")
}

// The server advertises no prompt today: the snapshot has none, and costs none.
func TestTheServerHasNoPromptToday(t *testing.T) {
	t.Parallel()

	snap, err := surface.Take(t.Context())
	require.NoError(t, err)
	assert.Empty(t, snap.Prompts)
	assert.Zero(t, snap.Summary().PromptsTotal)
}
