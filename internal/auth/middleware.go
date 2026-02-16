package auth

import (
	"context"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AuthenticationMiddleware returns a middleware function that checks for authentication errors
// before allowing tool operations to proceed
func AuthenticationMiddleware() server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Check if there's an authentication error in the context
			if authError, ok := ctx.Value(types.AuthErrorCtxKey).(error); ok && authError != nil {
				return nil, authError
			}
			return next(ctx, request)
		}
	}
}

// ResourceAuthenticationMiddleware returns a middleware function that checks for authentication errors
// before allowing resource operations to proceed
func ResourceAuthenticationMiddleware() server.ResourceHandlerMiddleware {
	return func(next server.ResourceHandlerFunc) server.ResourceHandlerFunc {
		return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			// Check if there's an authentication error in the context
			if authError, ok := ctx.Value(types.AuthErrorCtxKey).(error); ok && authError != nil {
				return nil, authError
			}
			return next(ctx, request)
		}
	}
}

// AuthenticatedPromptHandlerFunc wraps a prompt handler with authentication checking
func AuthenticatedPromptHandlerFunc(handler func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error)) func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		// Check if there's an authentication error in the context
		if authError, ok := ctx.Value(types.AuthErrorCtxKey).(error); ok && authError != nil {
			return nil, authError
		}
		return handler(ctx, request)
	}
}
