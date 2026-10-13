package execute_automation_test

import (
	"context"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// The responder runs on the Cortex the caller names, else on the server's default one.
func TestRunResponderSendsTheCortexIDAndParameters(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{}
	ctx := context.WithValue(workerContext(t, noWorkerNamedYAML, cortex), types.DefaultCortexIDCtxKey, "cortex-default")
	tool := &execute_automation.ExecuteAutomationTool{}

	_, err := tool.Handle(ctx, mcp.CallToolRequest{}, runResponder("Mailer_1_0"))
	require.NoError(t, err)

	named := runResponder("Mailer_1_0")
	named.CortexID = "cortex-named"
	named.Parameters = map[string]any{"to": "soc@example.com"}
	_, err = tool.Handle(ctx, mcp.CallToolRequest{}, named)
	require.NoError(t, err)

	actions := cortex.Actions()
	require.Len(t, actions, 2)
	assert.Equal(t, "cortex-default", actions[0].GetCortexId())
	assert.Empty(t, actions[0].Parameters)
	assert.Equal(t, "cortex-named", actions[1].GetCortexId())
	assert.Equal(t, map[string]any{"to": "soc@example.com"}, actions[1].Parameters)
}

// With permission filters, a target that cannot be checked against them is
// denied, not run.
func TestRunResponderIsDeniedWhenTheTargetScopeCannotBeChecked(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{Responders: []thehive.OutputWorker{responderMailer}}
	ctx := workerContext(t, `version: "1.0"
permissions:
  tools:
    execute-automation:
      allowed: true
      filters:
        _lte: {_field: "tlp", _value: 2}
  responders:
    mode: "block_list"
    blocked: []
`, cortex)

	err := (&execute_automation.ExecuteAutomationTool{}).ValidatePermissions(ctx, runResponder("Mailer_1_0"))
	require.ErrorContains(t, err, "failed to verify entity scope")
}
