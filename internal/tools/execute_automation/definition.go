package execute_automation

import (
	"github.com/mark3labs/mcp-go/mcp"
)

type ExecuteAutomationTool struct{}

func NewExecuteAutomationTool() *ExecuteAutomationTool {
	return &ExecuteAutomationTool{}
}

func (t *ExecuteAutomationTool) Definition() mcp.Tool {
	return mcp.NewTool(
		"execute-automation",
		mcp.WithDescription(`Execute Cortex analyzers and responders, or retrieve their execution status.

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
- For responders: provide action-id (not yet fully supported by API)

GETTING INFORMATION:
- List available analyzers: get-resource hive://metadata/automation/analyzers
- List available responders: get-resource hive://metadata/automation/responders?entityType=case&entityId=~123
- Read analyzer/responder documentation: get-resource hive://docs/automation/analyzers or hive://docs/automation/responders

EXAMPLES:
- Run analyzer: operation="run-analyzer", analyzer-id="VirusTotal_3_0", observable-id="~123456"
- Run responder: operation="run-responder", responder-id="Mailer_1_0", entity-type="case", entity-id="~789"
- Check job: operation="get-job-status", job-id="AWxyz123"`),
		mcp.WithString(
			"operation",
			mcp.Required(),
			mcp.Enum("run-analyzer", "run-responder", "get-job-status", "get-action-status"),
			mcp.Description("The operation to perform."),
		),
		mcp.WithString(
			"analyzer-id",
			mcp.Description("Analyzer ID for run-analyzer operations. Get available analyzers from hive://metadata/automation/analyzers"),
		),
		mcp.WithString(
			"responder-id",
			mcp.Description("Responder ID for run-responder operations. Get available responders from hive://metadata/automation/responders"),
		),
		mcp.WithString(
			"cortex-id",
			mcp.Description("Optional Cortex instance ID. If not specified, TheHive will automatically route to an available Cortex instance."),
		),
		mcp.WithString(
			"observable-id",
			mcp.Description("Observable (artifact) ID for run-analyzer operations. This is the entity being analyzed."),
		),
		mcp.WithString(
			"entity-type",
			mcp.Enum("case", "alert", "task", "observable"),
			mcp.Description("Entity type for run-responder operations."),
		),
		mcp.WithString(
			"entity-id",
			mcp.Description("Entity ID for run-responder operations. This is the entity the responder will act on."),
		),
		mcp.WithString(
			"job-id",
			mcp.Description("Job ID for get-job-status operations. Returned by run-analyzer."),
		),
		mcp.WithString(
			"action-id",
			mcp.Description("Action ID for get-action-status operations. Returned by run-responder."),
		),
		mcp.WithObject(
			"parameters",
			mcp.Description("Optional parameters for analyzer/responder execution. JSON object with automation-specific configuration."),
		),
	)
}
