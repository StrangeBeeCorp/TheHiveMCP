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

// EntityParamConstraints carries the enumerations that EntityParams can no
// longer express as struct tags, kept beside the definition so the permitted
// values sit next to the tool that accepts them.
//
// Exported so the schema tests can check every key against the struct: a
// constraint naming a field that no longer exists is otherwise a silent no-op.
var EntityParamConstraints = map[string]tools.SchemaConstraint{
	"operation": {Enum: []string{
		OperationCreate, OperationUpdate, OperationDelete, OperationComment,
		OperationPromote, OperationMerge, OperationApplyTemplate,
	}},
	"entity-type": {Enum: []string{
		"case", "alert", "task", "observable", "procedure", "case-template", "page",
	}},
}

// Definition returns the MCP tool definition including input and output schemas.
func (t *Tool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(ManageToolDescription),
		tools.WithInputSchemaConstraints[EntityParams](EntityParamConstraints),
		tools.WithUnionOutputSchema[EntityResult](),
		mcp.WithToolAnnotation(tools.MutatingToolAnnotation()),
	)
}
