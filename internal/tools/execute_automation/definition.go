// Package execute_automation implements the execute-automation MCP tool for
// running Cortex analyzers and responders and reading their execution status.
package execute_automation

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
)

// ExecuteAutomationTool exposes Cortex analyzer/responder execution and status
// retrieval as an MCP tool.
type ExecuteAutomationTool struct{}

// NewExecuteAutomationTool returns a new ExecuteAutomationTool.
func NewExecuteAutomationTool() *ExecuteAutomationTool {
	return &ExecuteAutomationTool{}
}

// Name returns the tool's registered MCP name.
func (t *ExecuteAutomationTool) Name() string {
	return tools.ToolNameExecuteAutomation
}

// Handler returns the tool handler wrapped with the shared validation middleware.
func (t *ExecuteAutomationTool) Handler() server.ToolHandlerFunc {
	return tools.WithValidation(t)
}

// HasUntrustedData reports that this tool's output may contain untrusted data.
func (t *ExecuteAutomationTool) HasUntrustedData() bool { return true }

// ExecuteAutomationParamConstraints carries the enumerations that
// ExecuteAutomationParams can no longer express as struct tags.
//
// Exported so the schema tests can check every key against the struct.
var ExecuteAutomationParamConstraints = map[string]tools.SchemaConstraint{
	"operation": {Enum: []string{
		OperationRunAnalyzer, OperationRunResponder, OperationGetJobStatus, OperationGetActionStatus,
	}},
	"entity-type": {Enum: []string{"case", "alert", "task", "observable"}},
}

// Definition returns the MCP tool definition (schema and description).
func (t *ExecuteAutomationTool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(ExecuteAutomationToolDescription),
		tools.WithInputSchemaConstraints[ExecuteAutomationParams](ExecuteAutomationParamConstraints),
		mcp.WithOutputSchema[ExecuteAutomationResult](),
	)
}
