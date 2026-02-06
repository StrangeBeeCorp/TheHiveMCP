package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Tool represents an MCP tool with its definition and handler
type Tool interface {
	Definition() mcp.Tool
	Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// Registry manages tool registration
type Registry struct {
	tools []Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make([]Tool, 0),
	}
}

func (r *Registry) Register(tool Tool) {
	r.tools = append(r.tools, tool)
}

func (r *Registry) RegisterAll(s *server.MCPServer) {
	for _, tool := range r.tools {
		def := tool.Definition()
		wrapped := withPermissionCheck(def.Name, tool.Handle)
		s.AddTool(def, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return wrapped(ctx, req)
		})
	}
}
