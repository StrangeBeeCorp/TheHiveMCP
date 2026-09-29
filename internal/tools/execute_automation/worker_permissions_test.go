package execute_automation_test

import (
	"context"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// Workers as TheHive lists them: an opaque ID and a name. The shipped analyst
// profile lists them by name.
var (
	analyzerVirusTotal  = thehive.OutputWorker{Id: "a0b1c2", Name: "VirusTotal_3_0", Version: "3.0", Description: "VirusTotal lookup"}
	responderDeleteCase = thehive.OutputWorker{Id: "d3e4f5", Name: "DeleteCase_1_0", Version: "1.0", Description: "Deletes the case"}
	responderMailer     = thehive.OutputWorker{Id: "a6b7c8", Name: "Mailer_1_0", Version: "1.0", Description: "Sends an email"}
)

const workersByNameYAML = `version: "1.0"
permissions:
  tools:
    execute-automation:
      allowed: true
  analyzers:
    mode: "allow_list"
    allowed: ["VirusTotal_3_0"]
  responders:
    mode: "block_list"
    blocked: ["DeleteCase_1_0"]
`

const noWorkerNamedYAML = `version: "1.0"
permissions:
  tools:
    execute-automation:
      allowed: true
  analyzers:
    mode: "allow_list"
    allowed: ["*"]
  responders:
    mode: "block_list"
    blocked: []
`

func workerContext(t *testing.T, rulesYAML string, cortex *testutils.FakeCortex) context.Context {
	t.Helper()

	perms, err := permissions.ParseYAML([]byte(rulesYAML))
	require.NoError(t, err)

	ctx := utils.AddPermissionsToContext(context.Background(), perms)

	return context.WithValue(ctx, types.HiveClientCtxKey, cortex.Client(t))
}

func runAnalyzer(analyzerID string) execute_automation.ExecuteAutomationParams {
	return execute_automation.ExecuteAutomationParams{
		Operation: execute_automation.OperationRunAnalyzer, AnalyzerID: analyzerID, ObservableID: "~2",
	}
}

func runResponder(responderID string) execute_automation.ExecuteAutomationParams {
	return execute_automation.ExecuteAutomationParams{
		Operation: execute_automation.OperationRunResponder, ResponderID: responderID, EntityType: types.EntityTypeCase, EntityID: "~1",
	}
}

// Regression: only the identifier the caller sent was checked, so a responder
// blocked by name ran when called by its ID.
func TestRunResponderBlockedByNameIsDeniedUnderEitherIdentifier(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{Responders: []thehive.OutputWorker{responderDeleteCase, responderMailer}}
	ctx := workerContext(t, workersByNameYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	for _, ref := range []string{responderDeleteCase.Id, responderDeleteCase.Name} {
		require.ErrorContains(t, tool.ValidatePermissions(ctx, runResponder(ref)), "is not permitted", ref)
	}

	require.NoError(t, tool.ValidatePermissions(ctx, runResponder(responderMailer.Id)))
}

// The same gap the other way round: an analyzer allowed by name was refused
// when called by its ID, the identifier the catalog tells the model to use.
func TestRunAnalyzerAllowedByNameIsAllowedUnderEitherIdentifier(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{Analyzers: []thehive.OutputWorker{analyzerVirusTotal}}
	ctx := workerContext(t, workersByNameYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	for _, ref := range []string{analyzerVirusTotal.Id, analyzerVirusTotal.Name} {
		require.NoError(t, tool.ValidatePermissions(ctx, runAnalyzer(ref)), ref)
	}
}

func TestRunWorkerMissingFromCatalogIsDenied(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{
		Analyzers:  []thehive.OutputWorker{analyzerVirusTotal},
		Responders: []thehive.OutputWorker{responderMailer},
	}
	ctx := workerContext(t, workersByNameYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	require.ErrorContains(t, tool.ValidatePermissions(ctx, runAnalyzer("Unknown_1_0")), "was not found")
	require.ErrorContains(t, tool.ValidatePermissions(ctx, runResponder("Unknown_1_0")), "was not found")
}

func TestRunWorkerIsDeniedWhenTheCatalogCannotBeRead(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{Down: true}
	ctx := workerContext(t, workersByNameYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	require.Error(t, tool.ValidatePermissions(ctx, runAnalyzer(analyzerVirusTotal.Id)))
	require.Error(t, tool.ValidatePermissions(ctx, runResponder(responderMailer.Id)))
}

// Rules that name no worker give every worker the same answer, so the catalog
// is not read: a Cortex that cannot list its workers must not start denying
// the runs a "*" allow list permits.
func TestRunWorkerSkipsTheCatalogWhenTheRulesNameNoWorker(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{Down: true}
	ctx := workerContext(t, noWorkerNamedYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	require.NoError(t, tool.ValidatePermissions(ctx, runAnalyzer("Any_1_0")))
	require.NoError(t, tool.ValidatePermissions(ctx, runResponder("Any_1_0")))
	require.Zero(t, cortex.Requests())
}
