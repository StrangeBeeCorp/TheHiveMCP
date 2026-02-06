package tools

import (
	"context"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func withPermissionCheck(toolName string, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		perms, err := utils.GetPermissionsFromContext(ctx)
		if err != nil {
			return NewToolError("failed to get permissions").Cause(err).Result()
		}
		if !perms.IsToolAllowed(toolName) {
			return NewToolErrorf("%s tool is not permitted", toolName).Result()
		}
		return next(ctx, req)
	}
}
