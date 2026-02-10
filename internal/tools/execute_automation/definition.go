package execute_automation

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type ExecuteAutomationTool struct{}

func NewExecuteAutomationTool() *ExecuteAutomationTool {
	return &ExecuteAutomationTool{}
}

func (t *ExecuteAutomationTool) Name() string {
	return tools.ToolNameExecuteAutomation
}

func (t *ExecuteAutomationTool) Handler() server.ToolHandlerFunc {
	return tools.WithValidation(t)
}

func (t *ExecuteAutomationTool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(ExecuteAutomationToolDescription),
		mcp.WithInputSchema[ExecuteAutomationParams](),
		mcp.WithOutputSchema[ExecuteAutomationResult](),
	)
}
