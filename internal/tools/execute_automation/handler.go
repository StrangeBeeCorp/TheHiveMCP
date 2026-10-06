package execute_automation

import (
	"context"
	"errors"
	"fmt"
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

	inputAction := thehive.NewInputAction(params.ResponderID, utils.ResponderObjectType(params.EntityType), params.EntityID)
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
const waitPollInterval = 2 * time.Second

// handleWaitForJobs polls TheHive for every job until all are final or the
// timeout passes, so the model spends one tool call instead of a turn per poll.
func (t *ExecuteAutomationTool) handleWaitForJobs(ctx context.Context, params ExecuteAutomationParams) (ExecuteAutomationResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError(errGetHiveClient).Cause(err).
			Hint(hintCheckAuth)
	}

	perms, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return ExecuteAutomationResult{}, tools.NewToolError("failed to get permissions").Cause(err)
	}

	interval := t.pollInterval
	if interval <= 0 {
		interval = waitPollInterval
	}

	start := time.Now()

	// The deadline bounds every scope check and read as well, so a stalled
	// TheHive cannot hold the call past the timeout that keeps it inside the
	// client's own.
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(params.TimeoutSecs)*time.Second)
	defer cancel()

	wait := newJobWait(params.JobIDs, perms.GetToolFilters(t.Name()))

	for {
		wait.poll(waitCtx, hiveClient)

		if wait.settled() || !sleep(waitCtx, interval) {
			break
		}
	}

	wait.markUnanswered(params.TimeoutSecs)

	if len(wait.jobs) == 0 {
		failure := tools.NewToolErrorf("failed to get the status of any of the %d jobs", len(wait.order)).
			Cause(errors.New(wait.errors[wait.order[0]])).
			Hint("Check that every job ID was returned by run-analyzer, and that TheHive is reachable")
		if params.timeoutCapped {
			failure = failure.Hintf("timeout-seconds was capped at the maximum, %d", MaxWaitSeconds)
		}

		return ExecuteAutomationResult{}, failure
	}

	waited := int(time.Since(start).Seconds())
	pending := wait.pending()
	failed := wait.failures()

	slog.Info("Waited for jobs", "jobs", len(wait.order), "pending", len(pending), "failed", len(failed), "seconds", waited)

	result := NewWaitForJobsResult(wait.read(), pending, failed, len(wait.order), waited)
	if params.timeoutCapped {
		result.Message = utils.Trustedf("timeout-seconds was capped at the maximum, %d. %s", MaxWaitSeconds, result.Message)
	}

	return ExecuteAutomationResult{WaitForJobsResult: result}, nil
}

// sleep waits for d, reporting false when ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// jobWait is the state of one wait-for-jobs call. Every job has its own
// outcome, so one bad job ID, one job outside the permitted scope or one
// transient error does not cost the other jobs their reports.
type jobWait struct {
	order   []string
	filters map[string]any                // the tool's permission filters; empty means every job is in scope
	scoped  map[string]bool               // jobs whose target passed the scope check
	jobs    map[string]*thehive.OutputJob // the latest successful read
	errors  map[string]string             // the latest failure, cleared by a later success
	gone    map[string]bool               // refused for good (out of scope, or a 4xx): not asked again
}

func newJobWait(jobIDs []string, filters map[string]any) *jobWait {
	return &jobWait{
		order:   jobIDs,
		filters: filters,
		scoped:  map[string]bool{},
		jobs:    make(map[string]*thehive.OutputJob, len(jobIDs)),
		errors:  map[string]string{},
		gone:    map[string]bool{},
	}
}

// poll re-reads every job whose state can still change, scope-checking each
// before its first read, as get-job-status does. It stops as soon as ctx ends:
// a job cut off by the deadline keeps its last read.
func (w *jobWait) poll(ctx context.Context, hiveClient *thehive.APIClient) {
	for _, jobID := range w.order {
		if w.final(jobID) {
			continue
		}

		if !w.scoped[jobID] && !w.checkScope(ctx, jobID) {
			if ctx.Err() != nil {
				return
			}

			continue
		}

		job, resp, err := hiveClient.CortexAPI.GetCortexJob(ctx, jobID).Execute()
		if ctx.Err() != nil {
			return
		}

		if err != nil {
			w.errors[jobID] = fmt.Sprintf("%v (%s)", err, utils.DescribeHTTPResponse(resp))
			w.gone[jobID] = utils.IsClientError(resp)

			continue
		}

		delete(w.errors, jobID)
		w.jobs[jobID] = job
	}
}

// checkScope reports whether the job may be read. A job outside the permitted
// scope gets the same answer as one that does not exist, so nothing leaks.
func (w *jobWait) checkScope(ctx context.Context, jobID string) bool {
	inScope, err := utils.IsJobObservableInScope(ctx, jobID, w.filters)

	switch {
	case ctx.Err() != nil:
		return false
	case err != nil:
		w.errors[jobID] = "failed to check the job against your permission filters: " + err.Error()
	case !inScope:
		w.errors[jobID] = fmt.Sprintf(errJobOutOfScope, jobID)
		w.gone[jobID] = true
	default:
		w.scoped[jobID] = true
	}

	return w.scoped[jobID]
}

// markUnanswered records a failure for every job the wait never managed to
// read, so a TheHive that does not answer is not reported as running jobs.
func (w *jobWait) markUnanswered(timeoutSecs int) {
	for _, jobID := range w.order {
		_, read := w.jobs[jobID]
		if _, failed := w.errors[jobID]; !read && !failed {
			w.errors[jobID] = fmt.Sprintf("TheHive did not answer within %ds", timeoutSecs)
		}
	}
}

// final reports whether polling the job again cannot change the answer.
func (w *jobWait) final(jobID string) bool {
	if w.gone[jobID] {
		return true
	}

	job, read := w.jobs[jobID]

	return read && isTerminal(job.GetStatus())
}

func (w *jobWait) settled() bool {
	for _, jobID := range w.order {
		if !w.final(jobID) {
			return false
		}
	}

	return true
}

// pending lists every job worth asking about again, in the caller's order:
// those last read as running, and those whose latest attempt failed in a way a
// later one may not, which failures also lists.
func (w *jobWait) pending() []string {
	var pending []string

	for _, jobID := range w.order {
		if !w.final(jobID) {
			pending = append(pending, jobID)
		}
	}

	return pending
}

// read returns every job read at least once, in the caller's order.
func (w *jobWait) read() []*thehive.OutputJob {
	jobs := make([]*thehive.OutputJob, 0, len(w.jobs))

	for _, jobID := range w.order {
		if job, read := w.jobs[jobID]; read {
			jobs = append(jobs, job)
		}
	}

	return jobs
}

// failures returns the jobs whose latest attempt failed, in the caller's order.
func (w *jobWait) failures() []JobError {
	var failures []JobError

	for _, jobID := range w.order {
		if msg, failed := w.errors[jobID]; failed {
			failures = append(failures, JobError{JobID: jobID, Error: msg, Retryable: !w.gone[jobID]})
		}
	}

	return failures
}

func isTerminal(status string) bool {
	_, terminal := terminalWorkerStatuses[status]

	return terminal
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
