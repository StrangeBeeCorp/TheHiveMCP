// Package manage implements the manage-entities MCP tool, exposing CRUD and
// workflow operations (create, update, delete, comment, promote, merge,
// apply-template) over TheHive entities.
package manage

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
)

// Tool is the MCP tool that performs CRUD and workflow operations on TheHive entities.
type Tool struct{}

// NewManageTool returns a new Tool.
func NewManageTool() *Tool {
	return &Tool{}
}

// Name returns the MCP tool name.
func (t *Tool) Name() string {
	return tools.ToolNameManageEntities
}

// Handler returns the MCP tool handler, wrapped with parameter and permission validation.
func (t *Tool) Handler() server.ToolHandlerFunc {
	return tools.WithValidation(t)
}

// HasUntrustedData reports that this tool's results may contain untrusted, user-generated data.
func (t *Tool) HasUntrustedData() bool { return true }

// Definition returns the MCP tool definition including input and output schemas.
func (t *Tool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(ManageToolDescription),
		mcp.WithInputSchema[EntityParams](),
		mcp.WithOutputSchema[EntityResult](),
	)
}
