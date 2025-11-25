package execute_automation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ExecuteAutomationTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract and validate parameters
	params, err := t.extractParams(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Validate operation constraints
	if err := t.validateOperation(params); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Execute operation
	switch params.Operation {
	case "run-analyzer":
		return t.handleRunAnalyzer(ctx, params)
	case "run-responder":
		return t.handleRunResponder(ctx, params)
	case "get-job-status":
		return t.handleGetJobStatus(ctx, params)
	case "get-action-status":
		return t.handleGetActionStatus(ctx, params)
	default:
		return mcp.NewToolResultError(fmt.Sprintf("unsupported operation: %s", params.Operation)), nil
	}
}

// Parameter extraction and validation
type executeAutomationParams struct {
	Operation    string
	AnalyzerID   string
	ResponderID  string
	CortexID     string
	ObservableID string
	EntityType   string
	EntityID     string
	JobID        string
	ActionID     string
	Parameters   map[string]interface{}
}

func (t *ExecuteAutomationTool) extractParams(req mcp.CallToolRequest) (*executeAutomationParams, error) {
	operation := req.GetString("operation", "")
	if operation == "" {
		return nil, fmt.Errorf("operation parameter is required. Must be one of: 'run-analyzer', 'run-responder', 'get-job-status', 'get-action-status'")
	}

	params := &executeAutomationParams{
		Operation:    operation,
		AnalyzerID:   req.GetString("analyzer-id", ""),
		ResponderID:  req.GetString("responder-id", ""),
		CortexID:     req.GetString("cortex-id", "local"),
		ObservableID: req.GetString("observable-id", ""),
		EntityType:   req.GetString("entity-type", ""),
		EntityID:     req.GetString("entity-id", ""),
		JobID:        req.GetString("job-id", ""),
		ActionID:     req.GetString("action-id", ""),
	}

	// Extract parameters if provided
	if parametersRaw := req.GetArguments()["parameters"]; parametersRaw != nil {
		if parametersMap, ok := parametersRaw.(map[string]interface{}); ok {
			params.Parameters = parametersMap
		} else {
			return nil, fmt.Errorf("parameters must be a valid JSON object")
		}
	}

	slog.Info("ExecuteAutomation called",
		"operation", params.Operation,
		"analyzerID", params.AnalyzerID,
		"responderID", params.ResponderID,
		"cortexID", params.CortexID)

	return params, nil
}

func (t *ExecuteAutomationTool) validateOperation(params *executeAutomationParams) error {
	switch params.Operation {
	case "run-analyzer":
		if params.AnalyzerID == "" {
			return fmt.Errorf("analyzer-id is required for run-analyzer operations. Get available analyzers from get-resource 'hive://metadata/automation/analyzers'")
		}
		if params.ObservableID == "" {
			return fmt.Errorf("observable-id is required for run-analyzer operations. This is the ID of the observable to analyze")
		}
	case "run-responder":
		if params.ResponderID == "" {
			return fmt.Errorf("responder-id is required for run-responder operations. Get available responders from get-resource 'hive://metadata/automation/responders?entityType=<type>&entityId=<id>'")
		}
		if params.EntityType == "" {
			return fmt.Errorf("entity-type is required for run-responder operations. Must be one of: 'case', 'alert', 'task', 'observable'")
		}
		if params.EntityID == "" {
			return fmt.Errorf("entity-id is required for run-responder operations. This is the ID of the entity the responder will act on")
		}
	case "get-job-status":
		if params.JobID == "" {
			return fmt.Errorf("job-id is required for get-job-status operations. Provide the job ID returned by run-analyzer")
		}
	case "get-action-status":
		if params.ActionID == "" {
			return fmt.Errorf("action-id is required for get-action-status operations. Provide the action ID returned by run-responder")
		}
	}
	return nil
}

// Run analyzer operation
func (t *ExecuteAutomationTool) handleRunAnalyzer(ctx context.Context, params *executeAutomationParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get TheHive client: %v. Check your authentication and connection settings.", err)), nil
	}

	// Create InputJob
	// Use empty string for cortexID if not provided - backend will auto-route
	inputJob := thehive.NewInputJob(params.AnalyzerID, params.CortexID, params.ObservableID)
	if params.Parameters != nil {
		inputJob.SetParameters(params.Parameters)
	}

	// Execute job
	job, resp, err := hiveClient.CortexAPI.CreateCortexJob(ctx).InputJob(*inputJob).Execute()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to execute analyzer: %v. Check that the analyzer ID, Cortex ID, and observable ID are correct. API response: %v", err, resp)), nil
	}

	slog.Info("Analyzer job created",
		"jobId", job.GetUnderscoreId(),
		"analyzerId", params.AnalyzerID,
		"status", job.GetStatus())

	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":    "run-analyzer",
		"analyzerId":   params.AnalyzerID,
		"analyzerName": job.GetAnalyzerName(),
		"job":          job,
		"message":      fmt.Sprintf("Analyzer job created successfully. Job ID: %s. Use get-job-status to check progress.", job.GetUnderscoreId()),
	})
}

// Run responder operation
func (t *ExecuteAutomationTool) handleRunResponder(ctx context.Context, params *executeAutomationParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get TheHive client: %v. Check your authentication and connection settings.", err)), nil
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to execute responder: %v. Check that the responder ID, entity type, and entity ID are correct. API response: %v", err, resp)), nil
	}

	slog.Info("Responder action created",
		"actionId", action.GetUnderscoreId(),
		"responderId", params.ResponderID,
		"status", action.GetStatus())

	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":     "run-responder",
		"responderId":   params.ResponderID,
		"responderName": action.GetResponderName(),
		"action":        action,
		"message":       fmt.Sprintf("Responder action created successfully. Action ID: %s. Status: %s", action.GetUnderscoreId(), action.GetStatus()),
	})
}

// Get job status operation
func (t *ExecuteAutomationTool) handleGetJobStatus(ctx context.Context, params *executeAutomationParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get TheHive client: %v. Check your authentication and connection settings.", err)), nil
	}

	// Get job status
	job, resp, err := hiveClient.CortexAPI.GetCortexJob(ctx, params.JobID).Execute()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get job status: %v. Check that the job ID is correct. API response: %v", err, resp)), nil
	}

	slog.Info("Job status retrieved",
		"jobId", params.JobID,
		"status", job.GetStatus(),
		"hasReport", job.HasReport())

	// Parse and format the result
	result := map[string]interface{}{
		"operation":    "get-job-status",
		"jobId":        params.JobID,
		"analyzerId":   job.GetAnalyzerId(),
		"analyzerName": job.GetAnalyzerName(),
		"status":       job.GetStatus(),
		"startDate":    job.GetStartDate(),
	}

	if job.HasEndDate() {
		result["endDate"] = job.GetEndDate()
	}

	if job.HasReport() {
		result["report"] = job.GetReport()
		result["message"] = fmt.Sprintf("Job completed with status: %s. Report available.", job.GetStatus())
	} else {
		result["message"] = fmt.Sprintf("Job status: %s. No report available yet.", job.GetStatus())
	}

	return utils.NewToolResultJSONUnescaped(result)
}

// Get action status operation (Note: TheHive API may have limited support for this)
func (t *ExecuteAutomationTool) handleGetActionStatus(ctx context.Context, params *executeAutomationParams) (*mcp.CallToolResult, error) {
	// Note: TheHive API doesn't have a direct GetAction endpoint like GetJob
	// We would need to query actions by entity to find this specific action
	// For now, return a helpful error message
	return mcp.NewToolResultError("get-action-status is not fully supported by TheHive API. To check responder actions, use search-entities to query the entity and check its actions, or use the CortexAPI.GetActionByEntity method with entity type and ID."), nil
}
