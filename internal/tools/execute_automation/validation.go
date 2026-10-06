package execute_automation

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// ValidatePermissions checks that the caller is permitted to run the requested
// operation and that the target entity is within the configured filter scope.
func (t *ExecuteAutomationTool) ValidatePermissions(ctx context.Context, params ExecuteAutomationParams) error {
	perms, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err)
	}

	if !perms.IsToolAllowed(t.Name()) {
		return tools.NewToolErrorf("tool %s is not permitted by your permissions configuration", t.Name())
	}

	switch params.Operation {
	case OperationRunAnalyzer:
		err = validateAnalyzer(ctx, perms, params.AnalyzerID)
		if err != nil {
			return err
		}

		return t.validateTargetScope(ctx, perms, types.EntityTypeObservable, params.ObservableID)
	case OperationRunResponder:
		err = validateResponder(ctx, perms, params.ResponderID, params.EntityType, params.EntityID)
		if err != nil {
			return err
		}

		return t.validateTargetScope(ctx, perms, params.EntityType, params.EntityID)
	case OperationGetActionStatus:
		// Status reads need no analyzer/responder permission, but the target
		// entity must still be within the configured filter scope.
		return t.validateTargetScope(ctx, perms, params.EntityType, params.EntityID)
	case OperationGetJobStatus:
		return t.validateJobScope(ctx, perms, params.JobID)
	case OperationWaitForJobs:
		// Scope-checked per job while waiting, under the wait's deadline, so a
		// job outside the permitted scope does not cost the others their reports.
		return nil
	default:
		return tools.NewToolErrorf("unsupported operation: %s", params.Operation)
	}
}

// errJobOutOfScope answers a job that does not exist and one outside the
// permitted scope alike, so the answer does not reveal which.
const errJobOutOfScope = "job %s was not found or its target is not within the scope permitted by your permissions configuration"

// validateAnalyzer denies an analyzer the rules exclude. The caller may name it
// by ID or by name and the rules may list either, so when the rules single out
// analyzers the catalog is read to learn the other identifier.
func validateAnalyzer(ctx context.Context, perms *permissions.Config, analyzerID string) error {
	if !perms.AnalyzerRulesNameWorkers() {
		if perms.IsAnalyzerAllowed(analyzerID) {
			return nil
		}

		return errWorkerNotPermitted(workerKindAnalyzer, analyzerID)
	}

	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError(errGetHiveClient).Cause(err).Hint(hintCheckAuth)
	}

	// The whole catalog: a windowed fetch could miss the analyzer and deny it.
	analyzers, resp, err := hiveClient.CortexAPI.ListAnalyzers(ctx).Range_("all").Execute()
	if err != nil {
		return tools.NewToolError("failed to look up the analyzer to check it against your permissions configuration").
			Cause(err).API(resp)
	}

	return checkWorker(workerKindAnalyzer, analyzerID, analyzers, perms.IsAnalyzerAllowed,
		"Use an analyzer from get-resource 'hive://metadata/automation/analyzers'")
}

// validateResponder is validateAnalyzer for responders, which TheHive lists
// per target entity.
func validateResponder(ctx context.Context, perms *permissions.Config, responderID, entityType, entityID string) error {
	if !perms.ResponderRulesNameWorkers() {
		if perms.IsResponderAllowed(responderID) {
			return nil
		}

		return errWorkerNotPermitted(workerKindResponder, responderID)
	}

	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError(errGetHiveClient).Cause(err).Hint(hintCheckAuth)
	}

	responders, resp, err := hiveClient.CortexAPI.ListResponders(ctx, utils.ResponderObjectType(entityType), entityID).Execute()
	if err != nil {
		return tools.NewToolError("failed to look up the responder to check it against your permissions configuration").
			Cause(err).API(resp)
	}

	return checkWorker(workerKindResponder, responderID, responders, perms.IsResponderAllowed,
		fmt.Sprintf("Use a responder from get-resource 'hive://metadata/automation/responders?entityType=%s&entityId=%s'", entityType, entityID))
}

const (
	workerKindAnalyzer  = "Analyzer"
	workerKindResponder = "Responder"
)

func errWorkerNotPermitted(kind, ref string) *tools.ToolError {
	return tools.NewToolErrorf("%s %s is not permitted by your permissions configuration", kind, ref)
}

// checkWorker denies ref unless every catalog worker it designates, by ID or
// by name, is allowed by the rules given both its identifiers. A ref matching
// nothing is denied too, since there is nothing to check it against.
func checkWorker(kind, ref string, catalog []thehive.OutputWorker, allowed func(identifiers ...string) bool, notFoundHint string) error {
	found := false

	for _, worker := range catalog {
		if worker.GetId() != ref && worker.GetName() != ref {
			continue
		}

		found = true

		if !allowed(worker.GetId(), worker.GetName()) {
			return errWorkerNotPermitted(kind, ref)
		}
	}

	if !found {
		return tools.NewToolErrorf("%s %s is not among the %d %ss TheHive lists, so it cannot be checked against your permissions configuration",
			kind, ref, len(catalog), strings.ToLower(kind)).
			Hint(notFoundHint).
			Hint("TheHive lists none when it cannot reach Cortex, and lists a worker enabled on several Cortex instances under one of its IDs only")
	}

	return nil
}

// validateJobScope denies job status reads when the job's target observable
// (resolved server-side) is excluded by the tool's filters (DL-6004).
func (t *ExecuteAutomationTool) validateJobScope(ctx context.Context, perms *permissions.Config, jobID string) error {
	permFilters := perms.GetToolFilters(t.Name())
	if len(permFilters) == 0 {
		return nil
	}

	inScope, err := utils.IsJobObservableInScope(ctx, jobID, permFilters)
	if err != nil {
		return tools.NewToolError("failed to verify entity scope").Cause(err).
			Hint("The operation was denied because the configured permission filters could not be checked against the job's target entity")
	}

	if !inScope {
		return tools.NewToolErrorf(errJobOutOfScope, jobID).
			Hint("The configured permission filters restrict which entities this tool can reach")
	}

	return nil
}

// validateTargetScope denies automation against entities excluded by the tool's
// filters (DL-6004). No filters means every target is permitted.
func (t *ExecuteAutomationTool) validateTargetScope(ctx context.Context, perms *permissions.Config, entityType, entityID string) error {
	permFilters := perms.GetToolFilters(t.Name())
	if len(permFilters) == 0 {
		return nil
	}

	// Tolerant get-by-idOrName resolution: the automation target id is user/LLM-supplied
	// and may be bare (not the ~-prefixed _id), which the batched _in{_id} query would
	// wrongly deny (DL-5764).
	inScope, err := utils.IsEntityInScopeTolerant(ctx, entityType, entityID, permFilters)
	if err != nil {
		return tools.NewToolError("failed to verify entity scope").Cause(err).
			Hint("The operation was denied because the configured permission filters could not be checked against the target entity")
	}

	if !inScope {
		return tools.NewToolErrorf("%s %s was not found or is not within the scope permitted by your permissions configuration", entityType, entityID).
			Hint("The configured permission filters restrict which entities this tool can reach")
	}

	return nil
}

// ValidateParams verifies that the parameters required by the requested
// operation are present.
func (t *ExecuteAutomationTool) ValidateParams(params *ExecuteAutomationParams) error {
	switch params.Operation {
	case OperationRunAnalyzer:
		return validateRunAnalyzerParams(params)
	case OperationRunResponder:
		return validateRunResponderParams(params)
	case OperationGetJobStatus:
		return validateGetJobStatusParams(params)
	case OperationGetActionStatus:
		return validateGetActionStatusParams(params)
	case OperationWaitForJobs:
		return validateWaitForJobsParams(params)
	default:
		return tools.NewToolErrorf("unsupported operation: %s", params.Operation)
	}
}

func validateRunAnalyzerParams(params *ExecuteAutomationParams) error {
	if params.AnalyzerID == "" {
		return tools.NewToolErrorf("analyzer-id is required for run-analyzer operations.").Hint("Get available analyzers from get-resource 'hive://metadata/automation/analyzers'")
	}

	if params.ObservableID == "" {
		return tools.NewToolErrorf("observable-id is required for run-analyzer operations. This is the ID of the observable to analyze")
	}

	return nil
}

func validateRunResponderParams(params *ExecuteAutomationParams) error {
	if params.ResponderID == "" {
		return tools.NewToolErrorf("responder-id is required for run-responder operations.").Hint("Get available responders from get-resource 'hive://metadata/automation/responders?entityType=<type>&entityId=<id>'")
	}

	if params.EntityType == "" {
		return tools.NewToolErrorf("entity-type is required for run-responder operations.").Hint("Must be one of: 'case', 'alert', 'task', 'observable'")
	}

	if params.EntityID == "" {
		return tools.NewToolErrorf("entity-id is required for run-responder operations. This is the ID of the entity the responder will act on")
	}

	return validateAutomationTarget(params)
}

func validateGetJobStatusParams(params *ExecuteAutomationParams) error {
	if params.JobID == "" {
		return tools.NewToolErrorf("job-id is required for get-job-status operations. Provide the job ID returned by run-analyzer")
	}

	return validateJobID("job-id", params.JobID)
}

// validateJobID holds a job ID to the shape of TheHive's job _id: get-job-status
// and wait-for-jobs put it in the path of the job endpoint.
func validateJobID(param, jobID string) error {
	if !automationEntityIDPattern.MatchString(jobID) {
		return tools.NewToolErrorf("invalid %s %q: must be a TheHive job ID such as ~123456", param, jobID).
			Hint("Use the jobId returned by run-analyzer (the job's _id, not its cortexJobId)")
	}

	return nil
}

func validateGetActionStatusParams(params *ExecuteAutomationParams) error {
	if params.ActionID == "" {
		return tools.NewToolErrorf("action-id is required for get-action-status operations.").Hint("Provide the action ID returned by run-responder")
	}

	if params.EntityType == "" {
		return tools.NewToolErrorf("entity-type is required for get-action-status operations.").Hint("Must be one of: 'case', 'alert', 'task', 'observable'")
	}

	if params.EntityID == "" {
		return tools.NewToolErrorf("entity-id is required for get-action-status operations. This is the ID of the entity the action is running against").Hint("Provide the entity ID the action is running against")
	}

	return validateAutomationTarget(params)
}

// automationEntityTypes are the entities a responder can act on, as the schema
// advertises them.
var automationEntityTypes = []string{types.EntityTypeCase, types.EntityTypeAlert, types.EntityTypeTask, types.EntityTypeObservable}

// entityTypeLog is a task log. Responders can act on one, but the schema does
// not advertise it, as nothing else in the tools does.
const entityTypeLog = "log"

// theHiveEntityTypes maps TheHive's own spellings, which a model may have seen
// in TheHive's answers, to the tools' names.
var theHiveEntityTypes = map[string]string{
	"case_task": types.EntityTypeTask, "case_artifact": types.EntityTypeObservable, "case_task_log": entityTypeLog,
}

// automationEntityIDPattern accepts a TheHive entity ID ("~" + digits) or the
// bare digits a model often sends (DL-5764). It admits no separator, dot or
// whitespace: run-responder and get-action-status put the value in a Cortex
// endpoint path, the kind RandoriSec 5.3 found open to traversal.
var automationEntityIDPattern = regexp.MustCompile(`^~?[0-9]+$`)

// validateAutomationTarget normalizes the entity type to an advertised name,
// accepting any case and TheHive's own spellings, and rejects a type or ID that
// could escape the path of the Cortex endpoint it is interpolated into. The
// schema advertises the enum, but nothing enforces a schema at runtime.
func validateAutomationTarget(params *ExecuteAutomationParams) error {
	entityType := strings.ToLower(params.EntityType)
	if name, isTheHiveName := theHiveEntityTypes[entityType]; isTheHiveName {
		entityType = name
	}

	if !slices.Contains(automationEntityTypes, entityType) && entityType != entityTypeLog {
		return tools.NewToolErrorf("invalid entity-type %q", params.EntityType).
			Hintf("Must be one of: %s", strings.Join(automationEntityTypes, ", "))
	}

	params.EntityType = entityType

	if !automationEntityIDPattern.MatchString(params.EntityID) {
		return tools.NewToolErrorf("invalid entity-id %q: must be a TheHive entity ID such as ~123456", params.EntityID)
	}

	return nil
}

// Bounds of a wait-for-jobs call. The ceiling keeps one blocking call inside a
// client's tool-call timeout; a caller with slower jobs calls again.
const (
	DefaultWaitSeconds = 45 // under the 60 s tool-call timeout of the MCP TypeScript SDK
	MaxWaitSeconds     = 120
	MaxWaitJobs        = 20
)

func validateWaitForJobsParams(params *ExecuteAutomationParams) error {
	if len(params.JobIDs) == 0 {
		return tools.NewToolErrorf("job-ids is required for wait-for-jobs operations. Provide the job IDs returned by run-analyzer")
	}

	seen := make(map[string]struct{}, len(params.JobIDs))
	unique := params.JobIDs[:0]

	for _, id := range params.JobIDs {
		if id == "" {
			return tools.NewToolErrorf("job-ids must not contain an empty ID")
		}

		err := validateJobID("job-ids entry", id)
		if err != nil {
			return err
		}

		if _, dup := seen[id]; !dup {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}

	params.JobIDs = unique

	if len(params.JobIDs) > MaxWaitJobs {
		return tools.NewToolErrorf("wait-for-jobs accepts at most %d job IDs, got %d", MaxWaitJobs, len(params.JobIDs)).
			Hint("Split the wait into several calls")
	}

	switch {
	case params.TimeoutSecs < 0:
		return tools.NewToolErrorf("timeout-seconds must be positive, got %d", params.TimeoutSecs)
	case params.TimeoutSecs == 0:
		params.TimeoutSecs = DefaultWaitSeconds
	case params.TimeoutSecs > MaxWaitSeconds:
		params.TimeoutSecs = MaxWaitSeconds
	}

	return nil
}
