// Package resource implements the get-resource MCP tool that exposes
// TheHive's hierarchical resources (schemas, metadata, docs, config) to clients.
package resource

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
)

// Tool is the get-resource MCP tool backed by a resource registry.
type Tool struct {
	resourceRegistry *resources.ResourceRegistry
}

// NewResourceTool creates a Tool serving resources from the given registry.
func NewResourceTool(registry *resources.ResourceRegistry) *Tool {
	return &Tool{
		resourceRegistry: registry,
	}
}

// Name returns the tool's registered name.
func (t *Tool) Name() string {
	return tools.ToolNameGetResource
}

// Handler returns the validated MCP handler for the tool.
func (t *Tool) Handler() server.ToolHandlerFunc {
	return tools.WithValidation[GetResourceParams, GetResourceResult](t)
}

// Definition returns the tool's MCP definition, including input and output schemas.
func (t *Tool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(GetResourceToolDescription),
		mcp.WithInputSchema[GetResourceParams](),
		mcp.WithOutputSchema[GetResourceResult](),
	)
}
