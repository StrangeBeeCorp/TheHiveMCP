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

type Tool[TParams, TResult any] interface {
	BaseTool
	Handle(ctx context.Context, request mcp.CallToolRequest, params TParams) (TResult, error)
	ValidateParams(params *TParams) error
	ValidatePermissions(ctx context.Context, params TParams) error
}

// UntrustedDataSource marks tools whose responses carry user-generated data. When
// true, the middleware wraps designated fields in [UNTRUSTED_DATA] tags so LLM
// clients can distinguish data from instructions.
type UntrustedDataSource interface {
	HasUntrustedData() bool
}

type Registry struct {
	tools []BaseTool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make([]BaseTool, 0),
	}
}

func (r *Registry) Register(tool BaseTool) {
	r.tools = append(r.tools, tool)
}

func (r *Registry) RegisterAll(s *server.MCPServer) {
	for _, tool := range r.tools {
		s.AddTool(tool.Definition(), tool.Handler())
	}
}

const (
	ToolNameManageEntities    = "manage-entities"
	ToolNameExecuteAutomation = "execute-automation"
	ToolNameGetResource       = "get-resource"
	ToolNameSearchEntities    = "search-entities"
)
