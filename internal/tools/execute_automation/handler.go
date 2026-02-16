package execute_automation

import (
	"context"
	"log/slog"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ExecuteAutomationTool) Handle(ctx context.Context, request mcp.CallToolRequest, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {

	switch params.Operation {
	case OperationRunAnalyzer:
		return t.handleRunAnalyzer(ctx, params)
	case OperationRunResponder:
		return t.handleRunResponder(ctx, params)
	case OperationGetJobStatus:
		return t.handleGetJobStatus(ctx, params)
	case OperationGetActionStatus:
		return t.handleGetActionStatus(ctx, params)
	default:
		return ExecuteAutomationResult{}, tools.NewToolErrorf("unsupported operation: %s", params.Operation)
	}
}

// Run analyzer operation
func (t *ExecuteAutomationTool) handleRunAnalyzer(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}
	// Create InputJob
	inputJob := thehive.NewInputJob(params.AnalyzerID, params.CortexID, params.ObservableID)
	if params.Parameters != nil {
		inputJob.SetParameters(params.Parameters)
	}

	// Execute job
	job, resp, err := hiveClient.CortexAPI.CreateCortexJob(ctx).InputJob(*inputJob).Execute()
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError("failed to execute analyzer").Cause(err).
			Hint("Check that the analyzer ID, Cortex ID, and observable ID are correct").API(resp)
	}
	slog.Info("Analyzer job created",
		"jobId", job.GetUnderscoreId(),
		"analyzerId", params.AnalyzerID,
		"status", job.GetStatus())

	analyzerJobResult := NewAnalyzerJobResult(job)
	return ExecuteAutomationResult{
		AnalyzerResult: analyzerJobResult,
	}, nil
}

// Run responder operation
func (t *ExecuteAutomationTool) handleRunResponder(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}
	// Create InputAction
	inputAction := thehive.NewInputAction(params.ResponderID, params.EntityType, params.EntityID)
	if params.CortexID != "" {
		inputAction.SetCortexId(params.CortexID)
	}
	if params.Parameters != nil {
		inputAction.SetParameters(params.Parameters)
	}

	// Execute action
	action, resp, err := hiveClient.CortexAPI.CreateAnAction(ctx).InputAction(*inputAction).Execute()
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError("failed to execute responder").Cause(err).
			Hint("Check that the responder ID, entity type, and entity ID are correct").API(resp)
	}

	slog.Info("Responder action created",
		"actionId", action.GetUnderscoreId(),
		"responderId", params.ResponderID,
		"status", action.GetStatus())

	responderResult := NewResponderActionResult(action)

	return ExecuteAutomationResult{
		ResponderResult: responderResult,
	}, nil
}

// Get job status operation
func (t *ExecuteAutomationTool) handleGetJobStatus(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	// Get job status
	job, resp, err := hiveClient.CortexAPI.GetCortexJob(ctx, params.JobID).Execute()
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError("failed to get job status").Cause(err).
			Hint("Check that the job ID is correct").API(resp)
	}

	slog.Info("Job status retrieved",
		"jobId", params.JobID,
		"status", job.GetStatus(),
		"hasReport", job.HasReport())

	jobStatusResult := NewAnalyzerJobStatusResult(job)

	return ExecuteAutomationResult{
		JobStatusResult: jobStatusResult,
	}, nil
}

// Get action status operation
func (t *ExecuteAutomationTool) handleGetActionStatus(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	actionList, resp, err := hiveClient.CortexAPI.GetActionByEntity(ctx, params.EntityType, params.EntityID).Execute()
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError("failed to get action status").Cause(err).
			Hint("Check that the entity type and entity ID are correct").API(resp)
	}

	var targetAction *thehive.OutputAction
	for _, action := range actionList {
		if action.GetUnderscoreId() == params.ActionID {
			targetAction = &action
			break
		}
	}

	if targetAction == nil {
		return ExecuteAutomationResult{}, tools.NewToolErrorf("action with ID %s not found for entity %s:%s", params.ActionID, params.EntityType, params.EntityID).
			Hint("Check that the action ID, entity type, and entity ID are correct")
	}

	slog.Info("Action status retrieved",
		"actionId", params.ActionID,
		"status", targetAction.GetStatus())

	cortexActionResult := NewResponderActionStatusResult(targetAction)
	return ExecuteAutomationResult{
		ActionStatusResult: cortexActionResult,
	}, nil
}
