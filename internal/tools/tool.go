package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// BaseTool is the non-generic interface for tool registration
type BaseTool interface {
	Name() string
	Definition() mcp.Tool
	Handler() server.ToolHandlerFunc
}

// Tool represents an MCP tool with typed parameters
type Tool[TParams, TResult any] interface {
	BaseTool
	Handle(ctx context.Context, request mcp.CallToolRequest, params TParams) (TResult, error)
	ValidateParams(params *TParams) error
	ValidatePermissions(ctx context.Context, params TParams) error
}

// UntrustedDataReporter is an optional interface implemented by tools whose
// responses carry user-generated data from external systems (e.g. TheHive). When
// implemented and returning true, the middleware wraps designated fields in
// [UNTRUSTED_DATA] boundary tags so LLM clients can distinguish data from
// instructions.
type UntrustedDataReporter interface {
	HasUntrustedData() bool
}

// Registry manages tool registration
type Registry struct {
	tools []BaseTool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make([]BaseTool, 0),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool BaseTool) {
	r.tools = append(r.tools, tool)
}

// RegisterAll registers every tool in the registry with the given MCP server.
func (r *Registry) RegisterAll(s *server.MCPServer) {
	for _, tool := range r.tools {
		s.AddTool(tool.Definition(), tool.Handler())
	}
}

// Tool name constants identify the MCP tools exposed by the server.
const (
	ToolNameManageEntities    = "manage-entities"
	ToolNameExecuteAutomation = "execute-automation"
	ToolNameGetResource       = "get-resource"
	ToolNameSearchEntities    = "search-entities"
)
