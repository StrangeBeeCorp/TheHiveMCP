package execute_automation_test

import (
	"context"
	"testing"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
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

	require.ErrorContains(t, tool.ValidatePermissions(ctx, runAnalyzer("Unknown_1_0")), "is not among the 1 analyzers TheHive lists")
	require.ErrorContains(t, tool.ValidatePermissions(ctx, runResponder("Unknown_1_0")), "is not among the 1 responders TheHive lists")
}

// TheHive answers an empty catalog, not an error, when it cannot reach
// Cortex: the denial must say so instead of claiming the worker is unknown.
func TestRunWorkerMissingFromAnEmptyCatalogSaysWhy(t *testing.T) {
	t.Parallel()

	ctx := workerContext(t, workersByNameYAML, &testutils.FakeCortex{})
	err := execute_automation.NewExecuteAutomationTool().ValidatePermissions(ctx, runAnalyzer(analyzerVirusTotal.Name))

	require.ErrorContains(t, err, "is not among the 0 analyzers TheHive lists")
	require.ErrorContains(t, err, "cannot reach Cortex")
}

// Both refs are ones the rules alone would let through (an analyzer allowed
// by name, a responder not blocked by ID), so only the lookup denies them.
func TestRunWorkerIsDeniedWhenTheCatalogCannotBeRead(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{Down: true}
	ctx := workerContext(t, workersByNameYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	require.ErrorContains(t, tool.ValidatePermissions(ctx, runAnalyzer(analyzerVirusTotal.Name)), "failed to look up the analyzer")
	require.ErrorContains(t, tool.ValidatePermissions(ctx, runResponder(responderMailer.Id)), "failed to look up the responder")
}

// A name can designate a worker on each of several Cortex instances; every
// one of them must be allowed, not only the first match.
func TestRunWorkerSharingItsNameMustBeAllowedOnEveryInstance(t *testing.T) {
	t.Parallel()

	otherInstance := responderDeleteCase
	otherInstance.Id = "e9f8a7"

	cortex := &testutils.FakeCortex{Responders: []thehive.OutputWorker{responderDeleteCase, otherInstance}}
	ctx := workerContext(t, `version: "1.0"
permissions:
  tools:
    execute-automation:
      allowed: true
  responders:
    mode: "allow_list"
    allowed: ["`+responderDeleteCase.Id+`"]
`, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	require.NoError(t, tool.ValidatePermissions(ctx, runResponder(responderDeleteCase.Id)))
	require.ErrorContains(t, tool.ValidatePermissions(ctx, runResponder(responderDeleteCase.Name)), "is not permitted")
}

// run-responder and get-action-status put the entity type and ID into a Cortex
// endpoint path, so a value that could leave that path is refused up front.
func TestAutomationTargetMustBeATheHiveEntity(t *testing.T) {
	t.Parallel()

	tool := execute_automation.NewExecuteAutomationTool()

	for _, operation := range []string{execute_automation.OperationRunResponder, execute_automation.OperationGetActionStatus} {
		valid := func(entityType, entityID string) error {
			params := execute_automation.ExecuteAutomationParams{
				Operation: operation, ResponderID: responderMailer.Id, ActionID: "~9", EntityType: entityType, EntityID: entityID,
			}

			return tool.ValidateParams(&params)
		}

		require.NoError(t, valid(types.EntityTypeCase, "~123456"), operation)
		require.NoError(t, valid(types.EntityTypeCase, "123456"), "%s: a bare ID stays accepted (DL-5764)", operation)
		require.ErrorContains(t, valid(types.EntityTypeCase, "~1/../../admin"), "invalid entity-id", operation)
		require.ErrorContains(t, valid(types.EntityTypeCase, "~1 "), "invalid entity-id", operation)
		require.ErrorContains(t, valid("case/../../user", "~1"), "invalid entity-type", operation)
	}

	for given, want := range map[string]string{
		"Task": types.EntityTypeTask, "case_task": types.EntityTypeTask, "case_artifact": types.EntityTypeObservable,
		"Log": "log", "case_task_log": "log",
	} {
		params := execute_automation.ExecuteAutomationParams{
			Operation: execute_automation.OperationRunResponder, ResponderID: responderMailer.Id, EntityType: given, EntityID: "~1",
		}
		require.NoError(t, tool.ValidateParams(&params), given)
		assert.Equal(t, want, params.EntityType, "%s is normalized to the advertised name", given)
	}
}

// Regression: TheHive's responder endpoints resolve case_task and
// case_artifact, not the task and observable the tool advertises, so both the
// catalog lookup and the run on a task answered 404.
func TestRunResponderOnATaskReachesIt(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{Responders: []thehive.OutputWorker{responderMailer}}
	ctx := workerContext(t, workersByNameYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	params := execute_automation.ExecuteAutomationParams{
		Operation: execute_automation.OperationRunResponder, ResponderID: responderMailer.Id, EntityType: types.EntityTypeTask, EntityID: "~7",
	}
	require.NoError(t, tool.ValidateParams(&params))
	require.NoError(t, tool.ValidatePermissions(ctx, params), "the catalog lookup must reach the task")

	_, err := tool.Handle(ctx, mcp.CallToolRequest{}, params)
	require.NoError(t, err)
	assert.Equal(t, []string{"case_task"}, cortex.ActionObjectTypes())
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
