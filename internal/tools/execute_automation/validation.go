package execute_automation

import (
	"context"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

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
		if !perms.IsAnalyzerAllowed(params.AnalyzerID) {
			return tools.NewToolErrorf("Analyzer %s is not permitted by your permissions configuration", params.AnalyzerID)
		}
		return t.validateTargetScope(ctx, perms, types.EntityTypeObservable, params.ObservableID)
	case OperationRunResponder:
		if !perms.IsResponderAllowed(params.ResponderID) {
			return tools.NewToolErrorf("Responder %s is not permitted by your permissions configuration", params.ResponderID)
		}
		return t.validateTargetScope(ctx, perms, params.EntityType, params.EntityID)
	case OperationGetActionStatus:
		// Status reads need no analyzer/responder permission, but the target
		// entity must still be within the configured filter scope.
		return t.validateTargetScope(ctx, perms, params.EntityType, params.EntityID)
	case OperationGetJobStatus:
		return t.validateJobScope(ctx, perms, params.JobID)
	default:
		return tools.NewToolErrorf("unsupported operation: %s", params.Operation)
	}
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
		return tools.NewToolErrorf("job %s was not found or its target is not within the scope permitted by your permissions configuration", jobID).
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

	inScope, err := utils.IsEntityInScope(ctx, entityType, entityID, permFilters)
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

func (t *ExecuteAutomationTool) ValidateParams(params *ExecuteAutomationParams) error {
	switch params.Operation {
	case OperationRunAnalyzer:
		if params.AnalyzerID == "" {
			return tools.NewToolErrorf("analyzer-id is required for run-analyzer operations.").Hint("Get available analyzers from get-resource 'hive://metadata/automation/analyzers'")
		}
		if params.ObservableID == "" {
			return tools.NewToolErrorf("observable-id is required for run-analyzer operations. This is the ID of the observable to analyze")
		}
	case OperationRunResponder:
		if params.ResponderID == "" {
			return tools.NewToolErrorf("responder-id is required for run-responder operations.").Hint("Get available responders from get-resource 'hive://metadata/automation/responders?entityType=<type>&entityId=<id>'")
		}
		if params.EntityType == "" {
			return tools.NewToolErrorf("entity-type is required for run-responder operations.").Hint("Must be one of: 'case', 'alert', 'task', 'observable'")
		}
		if params.EntityID == "" {
			return tools.NewToolErrorf("entity-id is required for run-responder operations. This is the ID of the entity the responder will act on")
		}
	case OperationGetJobStatus:
		if params.JobID == "" {
			return tools.NewToolErrorf("job-id is required for get-job-status operations. Provide the job ID returned by run-analyzer")
		}
	case OperationGetActionStatus:
		if params.ActionID == "" {
			return tools.NewToolErrorf("action-id is required for get-action-status operations.").Hint("Provide the action ID returned by run-responder")
		}
		if params.EntityType == "" {
			return tools.NewToolErrorf("entity-type is required for get-action-status operations.").Hint("Must be one of: 'case', 'alert', 'task', 'observable'")
		}
		if params.EntityID == "" {
			return tools.NewToolErrorf("entity-id is required for get-action-status operations. This is the ID of the entity the action is running against").Hint("Provide the entity ID the action is running against")
		}
	default:
		return tools.NewToolErrorf("unsupported operation: %s", params.Operation)
	}
	return nil
}
