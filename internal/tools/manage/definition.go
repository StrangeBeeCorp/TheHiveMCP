package manage

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type ManageTool struct{}

func NewManageTool() *ManageTool {
	return &ManageTool{}
}

func (t *ManageTool) Name() string {
	return tools.ToolNameManageEntities
}

func (t *ManageTool) Handler() server.ToolHandlerFunc {
	return tools.WithValidation(t)
}

func (t *ManageTool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(ManageToolDescription),
		mcp.WithInputSchema[ManageEntityParams](),
		mcp.WithOutputSchema[ManageEntityResult](),
	)
}
