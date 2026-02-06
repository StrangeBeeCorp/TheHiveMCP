package execute_automation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ExecuteAutomationTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 1. Check permissions
	perms, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err).Result()
	}

	if !perms.IsToolAllowed("execute-automation") {
		return tools.NewToolError("execute-automation tool is not permitted by your permissions configuration").Result()
	}

	// 2. Extract and validate parameters
	params, err := t.extractParams(req)
	if err != nil {
		return tools.NewToolError(err.Error()).Result()
	}

	// 3. Check analyzer/responder specific permissions
	switch params.Operation {
	case "run-analyzer":
		if !perms.IsAnalyzerAllowed(params.AnalyzerID) {
			return tools.NewToolErrorf("analyzer '%s' is not permitted by your permissions configuration", params.AnalyzerID).Result()
		}
	case "run-responder":
		if !perms.IsResponderAllowed(params.ResponderID) {
			return tools.NewToolErrorf("responder '%s' is not permitted by your permissions configuration", params.ResponderID).Result()
		}
	}

	// 4. Validate operation constraints
	if err := t.validateOperation(params); err != nil {
		return tools.NewToolError(err.Error()).Result()
	}

	// 5. Execute operation
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
		return tools.NewToolErrorf("unsupported operation: %s", params.Operation).Result()
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
		if params.EntityType == "" {
			return fmt.Errorf("entity-type is required for get-action-status operations. Must be one of: 'case', 'alert', 'task', 'observable'")
		}
		if params.EntityID == "" {
			return fmt.Errorf("entity-id is required for get-action-status operations. This is the ID of the entity the action is running against")
		}
	default:
		return fmt.Errorf("unsupported operation: %s", params.Operation)
	}
	return nil
}

// Run analyzer operation
func (t *ExecuteAutomationTool) handleRunAnalyzer(ctx context.Context, params *executeAutomationParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings").Result()
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
		return tools.NewToolError("failed to execute analyzer").Cause(err).
			Hint("Check that the analyzer ID, Cortex ID, and observable ID are correct").API(resp).Result()
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
	}), nil
}

// Run responder operation
func (t *ExecuteAutomationTool) handleRunResponder(ctx context.Context, params *executeAutomationParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings").Result()
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
		return tools.NewToolError("failed to execute responder").Cause(err).
			Hint("Check that the responder ID, entity type, and entity ID are correct").API(resp).Result()
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
	}), nil
}

// Get job status operation
func (t *ExecuteAutomationTool) handleGetJobStatus(ctx context.Context, params *executeAutomationParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings").Result()
	}

	// Get job status
	job, resp, err := hiveClient.CortexAPI.GetCortexJob(ctx, params.JobID).Execute()
	if err != nil {
		return tools.NewToolError("failed to get job status").Cause(err).
			Hint("Check that the job ID is correct").API(resp).Result()
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

	processedResult, err := utils.ParseDateFields(result)
	if err != nil {
		return tools.NewToolError("failed to parse date fields in job result").Cause(err).Result()
	}
	return utils.NewToolResultJSONUnescaped(processedResult), nil
}

// Get action status operation
func (t *ExecuteAutomationTool) handleGetActionStatus(ctx context.Context, params *executeAutomationParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings").Result()
	}

	actionList, resp, err := hiveClient.CortexAPI.GetActionByEntity(ctx, params.EntityType, params.EntityID).Execute()
	if err != nil {
		return tools.NewToolError("failed to get action status").Cause(err).
			Hint("Check that the entity type and entity ID are correct").API(resp).Result()
	}

	var targetAction *thehive.OutputAction
	for _, action := range actionList {
		if action.GetUnderscoreId() == params.ActionID {
			targetAction = &action
			break
		}
	}

	if targetAction == nil {
		return tools.NewToolErrorf("action with ID %s not found for entity %s:%s", params.ActionID, params.EntityType, params.EntityID).
			Hint("Check that the action ID, entity type, and entity ID are correct").Result()
	}

	slog.Info("Action status retrieved",
		"actionId", params.ActionID,
		"status", targetAction.GetStatus())

	result := map[string]interface{}{
		"operation":     "get-action-status",
		"actionId":      params.ActionID,
		"responderId":   targetAction.GetResponderId(),
		"responderName": targetAction.GetResponderName(),
		"startDate":     targetAction.GetStartDate(),
		"status":        targetAction.GetStatus(),
		"report":        targetAction.GetReport(),
	}

	if targetAction.HasEndDate() {
		result["endDate"] = targetAction.GetEndDate()
	}

	processedResult, err := utils.ParseDateFields(result)
	if err != nil {
		return tools.NewToolError("failed to parse date fields in action result").Cause(err).Result()
	}

	return utils.NewToolResultJSONUnescaped(processedResult), nil
}
