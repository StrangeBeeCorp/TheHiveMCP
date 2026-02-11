package search

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type SearchTool struct{}

func NewSearchTool() *SearchTool {
	return &SearchTool{}
}

func (t *SearchTool) Name() string {
	return tools.ToolNameSearchEntities
}

func (t *SearchTool) Handler() server.ToolHandlerFunc {
	return tools.WithValidation(t)
}

func (t *SearchTool) Definition() mcp.Tool {
	return mcp.NewTool(
		t.Name(),
		mcp.WithDescription(SearchEntitiesToolDescription),
		mcp.WithInputSchema[SearchEntitiesParams](),
		mcp.WithOutputSchema[SearchEntitiesResult](),
	)
}
