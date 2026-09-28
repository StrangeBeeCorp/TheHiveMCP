package execute_automation

import (
	"context"
	"log/slog"
	"time"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

const (
	errGetHiveClient = "failed to get TheHive client"
	hintCheckAuth    = "Check your authentication and connection settings"
)

// Handle dispatches the request to the operation-specific handler based on params.Operation.
func (t *ExecuteAutomationTool) Handle(ctx context.Context, _ mcp.CallToolRequest, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	if params.CortexID == "" {
		params.CortexID = utils.GetDefaultCortexIDFromContext(ctx)
	}

	switch params.Operation {
	case OperationRunAnalyzer:
		return t.handleRunAnalyzer(ctx, params)
	case OperationRunResponder:
		return t.handleRunResponder(ctx, params)
	case OperationGetJobStatus:
		return t.handleGetJobStatus(ctx, params)
	case OperationGetActionStatus:
		return t.handleGetActionStatus(ctx, params)
	case OperationWaitForJobs:
		return t.handleWaitForJobs(ctx, params)
	default:
		return ExecuteAutomationResult{}, tools.NewToolErrorf("unsupported operation: %s", params.Operation)
	}
}

func (t *ExecuteAutomationTool) handleRunAnalyzer(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError(errGetHiveClient).Cause(err).
			Hint(hintCheckAuth)
	}

	inputJob := thehive.NewInputJob(params.AnalyzerID, params.CortexID, params.ObservableID)
	if params.Parameters != nil {
		inputJob.SetParameters(params.Parameters)
	}

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

func (t *ExecuteAutomationTool) handleRunResponder(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError(errGetHiveClient).Cause(err).
			Hint(hintCheckAuth)
	}

	inputAction := thehive.NewInputAction(params.ResponderID, params.EntityType, params.EntityID)
	if params.CortexID != "" {
		inputAction.SetCortexId(params.CortexID)
	}

	if params.Parameters != nil {
		inputAction.SetParameters(params.Parameters)
	}

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

func (t *ExecuteAutomationTool) handleGetJobStatus(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError(errGetHiveClient).Cause(err).
			Hint(hintCheckAuth)
	}

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

// waitPollInterval is how often wait-for-jobs re-reads the jobs still running.
var waitPollInterval = 2 * time.Second

// handleWaitForJobs polls TheHive for every job until all are final or the
// timeout passes, so the model spends one tool call instead of a turn per poll.
func (t *ExecuteAutomationTool) handleWaitForJobs(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError(errGetHiveClient).Cause(err).
			Hint(hintCheckAuth)
	}

	start := time.Now()
	deadline := start.Add(time.Duration(params.TimeoutSecs) * time.Second)
	jobs := make(map[string]*thehive.OutputJob, len(params.JobIDs))

	for {
		pending, err := pollJobs(ctx, hiveClient, params.JobIDs, jobs)
		if err != nil {
			return ExecuteAutomationResult{}, err
		}

		if len(pending) == 0 || !time.Now().Add(waitPollInterval).Before(deadline) {
			return waitResult(params.JobIDs, jobs, pending, start), nil
		}

		select {
		case <-ctx.Done():
			return waitResult(params.JobIDs, jobs, pending, start), nil
		case <-time.After(waitPollInterval):
		}
	}
}

// pollJobs re-reads every job not yet final into jobs and returns the IDs still running.
func pollJobs(ctx context.Context, hiveClient *thehive.APIClient, jobIDs []string, jobs map[string]*thehive.OutputJob) ([]string, error) {
	pending := make([]string, 0, len(jobIDs))

	for _, jobID := range jobIDs {
		if job, seen := jobs[jobID]; seen && isTerminal(job.GetStatus()) {
			continue
		}

		job, resp, err := hiveClient.CortexAPI.GetCortexJob(ctx, jobID).Execute()
		if err != nil {
			return nil, tools.NewToolErrorf("failed to get status of job %s", jobID).Cause(err).
				Hint("Check that every job ID was returned by run-analyzer").API(resp)
		}

		jobs[jobID] = job
		if !isTerminal(job.GetStatus()) {
			pending = append(pending, jobID)
		}
	}

	return pending, nil
}

func isTerminal(status string) bool {
	_, terminal := terminalWorkerStatuses[status]

	return terminal
}

func waitResult(order []string, jobs map[string]*thehive.OutputJob, pending []string, start time.Time) ExecuteAutomationResult {
	ordered := make([]*thehive.OutputJob, 0, len(order))
	for _, jobID := range order {
		ordered = append(ordered, jobs[jobID])
	}

	slog.Info("Waited for jobs", "jobs", len(order), "pending", len(pending), "seconds", int(time.Since(start).Seconds()))

	return ExecuteAutomationResult{
		WaitForJobsResult: NewWaitForJobsResult(ordered, pending, int(time.Since(start).Seconds())),
	}
}

func (t *ExecuteAutomationTool) handleGetActionStatus(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError(errGetHiveClient).Cause(err).
			Hint(hintCheckAuth)
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
