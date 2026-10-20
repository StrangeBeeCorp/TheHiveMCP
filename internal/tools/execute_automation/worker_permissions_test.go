package execute_automation_test

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/testutils"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools/execute_automation"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// Kinds of worker, as the subtests of the answer tests name them.
const (
	workerResponder = "responder"
	workerAnalyzer  = "analyzer"
)

// contentTypeHTML is what a proxy or a login page answers with instead of JSON.
const contentTypeHTML = "text/html"

// Workers as TheHive lists them: an opaque ID and a name. Each is a recorded
// catalog entry with the fields the test cares about overridden. The shipped
// analyst profile lists them by name.
func analyzerVirusTotal(t *testing.T) testutils.Worker {
	t.Helper()

	return testutils.RecordedAnalyzer(t, "VirusTotal_GetReport_3_1").With("name", "VirusTotal_3_0")
}

func responderDeleteCase(t *testing.T) testutils.Worker {
	t.Helper()

	return testutils.RecordedResponder(t, "ScriptRunner_1_0").With("name", "DeleteCase_1_0")
}

func responderMailer(t *testing.T) testutils.Worker {
	t.Helper()

	return testutils.RecordedResponder(t, "Mailer_1_0")
}

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

	cortex := &testutils.FakeCortex{Responders: []testutils.Worker{responderDeleteCase(t), responderMailer(t)}}
	ctx := workerContext(t, workersByNameYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	for _, ref := range []string{responderDeleteCase(t).ID(), responderDeleteCase(t).Name()} {
		require.ErrorContains(t, tool.ValidatePermissions(ctx, runResponder(ref)), "is not permitted", ref)
	}

	require.NoError(t, tool.ValidatePermissions(ctx, runResponder(responderMailer(t).ID())))
}

// The same gap the other way round: an analyzer allowed by name was refused
// when called by its ID, the identifier the catalog tells the model to use.
func TestRunAnalyzerAllowedByNameIsAllowedUnderEitherIdentifier(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{Analyzers: []testutils.Worker{analyzerVirusTotal(t)}}
	ctx := workerContext(t, workersByNameYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	for _, ref := range []string{analyzerVirusTotal(t).ID(), analyzerVirusTotal(t).Name()} {
		require.NoError(t, tool.ValidatePermissions(ctx, runAnalyzer(ref)), ref)
	}
}

func TestRunWorkerMissingFromCatalogIsDenied(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{
		Analyzers:  []testutils.Worker{analyzerVirusTotal(t)},
		Responders: []testutils.Worker{responderMailer(t)},
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
	err := execute_automation.NewExecuteAutomationTool().ValidatePermissions(ctx, runAnalyzer(analyzerVirusTotal(t).Name()))

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

	require.ErrorContains(t, tool.ValidatePermissions(ctx, runAnalyzer(analyzerVirusTotal(t).Name())), "failed to look up the analyzer")
	require.ErrorContains(t, tool.ValidatePermissions(ctx, runResponder(responderMailer(t).ID())), "failed to look up the responder")
}

// A name can designate a worker on each of several Cortex instances; every
// one of them must be allowed, not only the first match.
func TestRunWorkerSharingItsNameMustBeAllowedOnEveryInstance(t *testing.T) {
	t.Parallel()

	otherInstance := responderDeleteCase(t).With("id", "e9f8a7")

	cortex := &testutils.FakeCortex{Responders: []testutils.Worker{responderDeleteCase(t), otherInstance}}
	ctx := workerContext(t, `version: "1.0"
permissions:
  tools:
    execute-automation:
      allowed: true
  responders:
    mode: "allow_list"
    allowed: ["`+responderDeleteCase(t).ID()+`"]
`, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	require.NoError(t, tool.ValidatePermissions(ctx, runResponder(responderDeleteCase(t).ID())))
	require.ErrorContains(t, tool.ValidatePermissions(ctx, runResponder(responderDeleteCase(t).Name())), "is not permitted")
}

// run-responder and get-action-status put the entity type and ID into a Cortex
// endpoint path, so a value that could leave that path is refused up front.
func TestAutomationTargetMustBeATheHiveEntity(t *testing.T) {
	t.Parallel()

	tool := execute_automation.NewExecuteAutomationTool()

	for _, operation := range []string{execute_automation.OperationRunResponder, execute_automation.OperationGetActionStatus} {
		valid := func(entityType, entityID string) error {
			params := execute_automation.ExecuteAutomationParams{
				Operation: operation, ResponderID: responderMailer(t).ID(), ActionID: "~9", EntityType: entityType, EntityID: entityID,
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
			Operation: execute_automation.OperationRunResponder, ResponderID: responderMailer(t).ID(), EntityType: given, EntityID: "~1",
		}
		require.NoError(t, tool.ValidateParams(&params), given)
		assert.Equal(t, want, params.EntityType, "%s is normalized to the advertised name", given)
	}
}

// Regression: get-job-status and wait-for-jobs put job IDs into the job
// endpoint's path unchecked; the 1.2.0 test run reached a reverse proxy with
// "../../user/current".
func TestJobIDsMustBeTheHiveJobIDs(t *testing.T) {
	t.Parallel()

	tool := execute_automation.NewExecuteAutomationTool()
	getStatus := func(jobID string) error {
		return tool.ValidateParams(&execute_automation.ExecuteAutomationParams{Operation: execute_automation.OperationGetJobStatus, JobID: jobID})
	}
	wait := func(jobIDs ...string) error {
		return tool.ValidateParams(&execute_automation.ExecuteAutomationParams{Operation: execute_automation.OperationWaitForJobs, JobIDs: jobIDs})
	}

	require.NoError(t, getStatus("~4096"))
	require.NoError(t, getStatus("4096"), "a bare ID stays accepted, as for entities")
	require.NoError(t, wait("~4096", "~8192"))

	for _, bad := range []string{"../../user/current", "..", "~1/../../../status", "~1%2F..", "AYx-cortexJobId", "~1 "} {
		require.ErrorContains(t, getStatus(bad), "invalid job-id", bad)
		require.ErrorContains(t, wait("~4096", bad), "invalid job-ids entry", bad)
	}
}

// Regression: TheHive's responder endpoints resolve case_task and
// case_artifact, not the task and observable the tool advertises, so both the
// catalog lookup and the run on a task answered 404.
func TestRunResponderOnATaskReachesIt(t *testing.T) {
	t.Parallel()

	cortex := &testutils.FakeCortex{Responders: []testutils.Worker{responderMailer(t)}}
	ctx := workerContext(t, workersByNameYAML, cortex)
	tool := execute_automation.NewExecuteAutomationTool()

	params := execute_automation.ExecuteAutomationParams{
		Operation: execute_automation.OperationRunResponder, ResponderID: responderMailer(t).ID(), EntityType: types.EntityTypeTask, EntityID: "~7",
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
