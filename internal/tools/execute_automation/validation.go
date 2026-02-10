package execute_automation

import (
	"context"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func (t *ExecuteAutomationTool) ValidatePermissions(ctx context.Context, params ExecuteAutomationParams) error {
	permissions, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err)
	}

	if !permissions.IsToolAllowed(t.Name()) {
		return tools.NewToolErrorf("tool %s is not permitted by your permissions configuration", t.Name())
	}

	switch params.Operation {
	case OperationRunAnalyzer:
		if !permissions.IsAnalyzerAllowed(params.AnalyzerID) {
			return tools.NewToolErrorf("Analyzer %s is not permitted by your permissions configuration", params.AnalyzerID)
		}
	case OperationRunResponder:
		if !permissions.IsResponderAllowed(params.ResponderID) {
			return tools.NewToolErrorf("Responder %s is not permitted by your permissions configuration", params.ResponderID)
		}
	case OperationGetJobStatus, OperationGetActionStatus:
		// Assuming that if the user can run analyzers/responders, they can check status. Adjust if needed.
		return nil
	default:
		return tools.NewToolErrorf("unsupported operation: %s", params.Operation)
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
