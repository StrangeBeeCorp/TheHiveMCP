package execute_automation

import (
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

const ExecuteAutomationToolDescription = `Execute Cortex analyzers and responders, or retrieve their execution status.

OPERATIONS:
- run-analyzer: Execute an analyzer on an observable (artifact) to enrich it with additional information
- run-responder: Execute a responder on an entity (case, alert, task, observable) to perform an action
- get-job-status: Retrieve the status and results of an analyzer job
- get-action-status: Retrieve the status and results of a responder action

ANALYZER EXECUTION:
Analyzers enrich observables by querying external services (threat intel, reputation, etc.).
- Requires: analyzer-id, observable-id
- Optional: cortex-id (auto-routed if not specified), parameters (JSON object with analyzer-specific configuration)
- Returns: OutputJob with job ID for status tracking

RESPONDER EXECUTION:
Responders perform active responses on entities (block IP, send email, create ticket, etc.).
- Requires: responder-id, entity-type, entity-id
- Optional: cortex-id (auto-routed if not specified), parameters (JSON object with responder-specific configuration)
- Returns: OutputAction with action ID for status tracking

STATUS RETRIEVAL:
Check the execution status and retrieve results of jobs or actions.
- For analyzers: provide job-id
- For responders: provide action-id, entity-type, and entity-id to find the specific action on that entity

GETTING INFORMATION:
- List available analyzers: get-resource hive://metadata/automation/analyzers
- List available responders: get-resource hive://metadata/automation/responders?entityType=case&entityId=~123
- Read analyzer/responder documentation: get-resource hive://docs/automation/analyzers or hive://docs/automation/responders

EXAMPLES:
- Run analyzer: operation="run-analyzer", analyzer-id="VirusTotal_3_0", observable-id="~123456"
- Run responder: operation="run-responder", responder-id="Mailer_1_0", entity-type="case", entity-id="~789"
- Check job: operation="get-job-status", job-id="AWxyz123"`

// Parameter extraction and validation
type ExecuteAutomationParams struct {
	Operation    string                 `json:"operation" jsonschema:"enum=run-analyzer,enum=run-responder,enum=get-job-status,enum=get-action-status,required=true" jsonschema_description:"The operation to perform."`
	AnalyzerID   string                 `json:"analyzer-id,omitempty" jsonschema_description:"Analyzer ID for run-analyzer operations. Get available analyzers from hive://metadata/automation/analyzers"`
	ResponderID  string                 `json:"responder-id,omitempty" jsonschema_description:"Responder ID for run-responder operations. Get available responders from hive://metadata/automation/responders"`
	CortexID     string                 `json:"cortex-id,omitempty" jsonschema:"default=local" jsonschema_description:"Cortex instance ID to run the analyzer or responder on. If not specified, the system will auto-route to an appropriate Cortex based on the analyzer/responder configuration and availability."`
	ObservableID string                 `json:"observable-id,omitempty" jsonschema_description:"Observable (artifact) ID for run-analyzer operations. This is the entity being analyzed."`
	EntityType   string                 `json:"entity-type,omitempty" jsonschema:"enum=case,enum=alert,enum=task,enum=observable" jsonschema_description:"Entity type for run-responder operations."`
	EntityID     string                 `json:"entity-id,omitempty" jsonschema_description:"Entity ID for run-responder operations. This is the specific entity the responder will act upon."`
	JobID        string                 `json:"job-id,omitempty" jsonschema_description:"Job ID for get-job-status operations."`
	ActionID     string                 `json:"action-id,omitempty" jsonschema_description:"Action ID for get-action-status operations."`
	Parameters   map[string]interface{} `json:"parameters,omitempty" jsonschema_description:"Optional parameters for analyzer/responder execution. JSON object with automation-specific configuration."`
}

const (
	OperationRunAnalyzer     = "run-analyzer"
	OperationRunResponder    = "run-responder"
	OperationGetJobStatus    = "get-job-status"
	OperationGetActionStatus = "get-action-status"
)

type FilteredOutputJob struct {
	UnderscoreId string                 `json:"_id"`
	AnalyzerId   string                 `json:"analyzerId"`
	AnalyzerName string                 `json:"analyzerName"`
	Status       string                 `json:"status"`
	StartDate    int64                  `json:"startDate"`
	EndDate      int64                  `json:"endDate,omitempty"`
	Report       map[string]interface{} `json:"report,omitempty"`
	CortexId     string                 `json:"cortexId"`
	CortexJobId  string                 `json:"cortexJobId"`
}

func NewFilteredOutputJob(job *thehive.OutputJob) *FilteredOutputJob {
	return &FilteredOutputJob{
		UnderscoreId: job.GetUnderscoreId(),
		AnalyzerId:   job.GetAnalyzerId(),
		AnalyzerName: job.GetAnalyzerName(),
		Status:       job.GetStatus(),
		StartDate:    job.GetStartDate(),
		EndDate:      job.GetEndDate(),
		Report:       job.GetReport(),
		CortexId:     job.GetCortexId(),
		CortexJobId:  job.GetCortexJobId(),
	}
}

type AnalyzerJobResult struct {
	Operation string             `json:"operation"`
	Job       *FilteredOutputJob `json:"job"`
	Message   string             `json:"message"`
}

func NewAnalyzerJobResult(job *thehive.OutputJob) *AnalyzerJobResult {
	return &AnalyzerJobResult{
		Operation: OperationRunAnalyzer,
		Job:       NewFilteredOutputJob(job),
		Message:   fmt.Sprintf("Analyzer job created successfully. Job ID: %s. Use get-job-status to check progress.", job.GetUnderscoreId()),
	}
}

type FilteredOutputAction struct {
	UnderscoreId  string `json:"_id"`
	ResponderId   string `json:"responderId"`
	ResponderName string `json:"responderName,omitempty"`
	CortexId      string `json:"cortexId,omitempty"`
	CortexJobId   string `json:"cortexJobId,omitempty"`
	ObjectType    string `json:"objectType"`
	ObjectId      string `json:"objectId"`
	Status        string `json:"status"`
	StartDate     int64  `json:"startDate"`
	EndDate       int64  `json:"endDate,omitempty"`
}

func NewFilteredOutputAction(action *thehive.OutputAction) *FilteredOutputAction {
	return &FilteredOutputAction{
		UnderscoreId:  action.GetUnderscoreId(),
		ResponderId:   action.GetResponderId(),
		ResponderName: action.GetResponderName(),
		CortexId:      action.GetCortexId(),
		CortexJobId:   action.GetCortexJobId(),
		ObjectType:    action.GetObjectType(),
		ObjectId:      action.GetObjectId(),
		Status:        action.GetStatus(),
		StartDate:     action.GetStartDate(),
		EndDate:       action.GetEndDate(),
	}
}

type ResponderActionResult struct {
	Operation string                `json:"operation"`
	Action    *FilteredOutputAction `json:"action"`
	Message   string                `json:"message"`
}

func NewResponderActionResult(action *thehive.OutputAction) *ResponderActionResult {
	return &ResponderActionResult{
		Operation: OperationRunResponder,
		Action:    NewFilteredOutputAction(action),
		Message:   fmt.Sprintf("Responder action created successfully. Action ID: %s. Status: %s", action.GetUnderscoreId(), action.GetStatus()),
	}
}

type AnalyzerJobStatusResult struct {
	Operation    string                 `json:"operation"`
	JobID        string                 `json:"jobId"`
	AnalyzerID   string                 `json:"analyzerId"`
	AnalyzerName string                 `json:"analyzerName"`
	Status       string                 `json:"status"`
	Result       map[string]interface{} `json:"result,omitempty"`
	Message      string                 `json:"message"`
}

func NewAnalyzerJobStatusResult(job *thehive.OutputJob) *AnalyzerJobStatusResult {
	result := &AnalyzerJobStatusResult{
		Operation:    OperationGetJobStatus,
		JobID:        job.GetUnderscoreId(),
		AnalyzerID:   job.GetAnalyzerId(),
		AnalyzerName: job.GetAnalyzerName(),
		Status:       job.GetStatus(),
		Message:      fmt.Sprintf("Job status: %s. Use get-job-status to check for updates.", job.GetStatus()),
	}

	if job.HasReport() {
		result.Result = job.GetReport()
		result.Message = fmt.Sprintf("Job completed with status: %s. Report available.", job.GetStatus())
	}

	return result
}

type ResponderActionStatusResult struct {
	Operation     string `json:"operation"`
	ActionID      string `json:"actionId"`
	ResponderID   string `json:"responderId"`
	ResponderName string `json:"responderName"`
	EntityType    string `json:"entityType"`
	EntityID      string `json:"entityId"`
	Status        string `json:"status"`
	Result        string `json:"result,omitempty"`
	Message       string `json:"message"`
}

func NewResponderActionStatusResult(action *thehive.OutputAction) *ResponderActionStatusResult {
	return &ResponderActionStatusResult{
		Operation:     OperationGetActionStatus,
		ActionID:      action.GetUnderscoreId(),
		ResponderID:   action.GetResponderId(),
		ResponderName: action.GetResponderName(),
		EntityType:    action.GetObjectType(),
		EntityID:      action.GetObjectId(),
		Status:        action.GetStatus(),
		Result:        action.GetReport(),
		Message:       fmt.Sprintf("Action status: %s. Use get-action-status to check for updates.", action.GetStatus()),
	}
}

// Union type for different operation results
type ExecuteAutomationResult struct {
	AnalyzerResult     *AnalyzerJobResult           `json:"analyzerResult,omitempty"`
	ResponderResult    *ResponderActionResult       `json:"responderResult,omitempty"`
	JobStatusResult    *AnalyzerJobStatusResult     `json:"jobStatusResult,omitempty"`
	ActionStatusResult *ResponderActionStatusResult `json:"actionStatusResult,omitempty"`
}

// Unwrap implements utils.Unwrapper to flatten the union for serialization.
func (r ExecuteAutomationResult) Unwrap() any { return utils.UnwrapUnion(r) }
