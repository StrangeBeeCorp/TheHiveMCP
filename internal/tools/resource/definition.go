package resource

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type ResourceTool struct {
	resourceRegistry *resources.ResourceRegistry
}

func NewResourceTool(registry *resources.ResourceRegistry) *ResourceTool {
	return &ResourceTool{
		resourceRegistry: registry,
	}
}

func (t *ResourceTool) Name() string {
	return tools.ToolNameGetResource
}

func (t *ResourceTool) Handler() server.ToolHandlerFunc {
	return tools.WithValidation[GetResourceParams, GetResourceResult](t)
}

func (t *ResourceTool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(GetResourceToolDescription),
		mcp.WithInputSchema[GetResourceParams](),
		mcp.WithOutputSchema[GetResourceResult](),
	)
}
