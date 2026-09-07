// Package search implements the MCP search tool that queries TheHive entities
// using the TheHive filter DSL.
package search

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
)

// Tool is the MCP tool that searches TheHive entities.
type Tool struct{}

// NewSearchTool returns a new Tool.
func NewSearchTool() *Tool {
	return &Tool{}
}

// Name returns the MCP tool name.
func (t *Tool) Name() string {
	return tools.ToolNameSearchEntities
}

// Handler returns the validated MCP tool handler.
func (t *Tool) Handler() server.ToolHandlerFunc {
	return tools.WithValidation(t)
}

// HasUntrustedData reports that results may contain untrusted user data.
func (t *Tool) HasUntrustedData() bool { return true }

// entitiesParamConstraints carries the enumerations and defaults that
// EntitiesParams can no longer express as struct tags.
var entitiesParamConstraints = map[string]tools.SchemaConstraint{
	"entity-type": {Enum: []string{
		"alert", "case", "task", "observable", "procedure", "pattern", "case-template", "page",
	}},
	"sort-by":    {Default: "_createdAt"},
	"sort-order": {Enum: []string{SortOrderAsc, SortOrderDesc}, Default: SortOrderDesc},
	"limit":      {Default: DefaultSearchLimit},
}

// Definition returns the MCP tool definition.
func (t *Tool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(SearchEntitiesToolDescription),
		tools.WithInputSchemaConstraints[EntitiesParams](entitiesParamConstraints),
		mcp.WithOutputSchema[EntitiesResult](),
	)
}
