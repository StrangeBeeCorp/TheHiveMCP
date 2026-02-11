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

// Registry manages tool registration
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
