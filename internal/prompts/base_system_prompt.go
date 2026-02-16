package prompts

import (
	"context"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func GetBaseSystemPrompt(ctx context.Context) (*mcp.GetPromptResult, error) {
	config := PromptConfig{
		TemplateName: "base_system_prompt.tmpl",
		Title:        "Base System Prompt",
	}
	return ProcessPromptWithExamples(ctx, config)
}

func RegisterBaseSystemPromptHandler(s *server.MCPServer) {
	baseSystemPrompt := mcp.NewPrompt(
		"base-system-prompt",
		mcp.WithPromptDescription("Base system prompt for the Hivemind."),
	)
	handler := func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return GetBaseSystemPrompt(ctx)
	}

	s.AddPrompt(baseSystemPrompt, auth.AuthenticatedPromptHandlerFunc(handler))

}
