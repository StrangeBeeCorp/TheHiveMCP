package auth

import (
	"context"
	"errors"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ErrAuthenticationNotValidated is returned when a request reaches a handler
// without the authentication marker set by the transport context function.
var ErrAuthenticationNotValidated = errors.New("TheHive authentication failed: credentials were not validated")

// checkAuthentication enforces fail-closed auth: allowed only when the context
// recorded a successful validation (types.AuthValidatedCtxKey) and no auth
// error is present. A context error takes precedence over the missing-marker error.
func checkAuthentication(ctx context.Context) error {
	if authError, ok := ctx.Value(types.AuthErrorCtxKey).(error); ok && authError != nil {
		return authError
	}
	if validated, ok := ctx.Value(types.AuthValidatedCtxKey).(bool); !ok || !validated {
		return ErrAuthenticationNotValidated
	}
	return nil
}

func AuthenticationMiddleware() server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := checkAuthentication(ctx); err != nil {
				return nil, err
			}
			return next(ctx, request)
		}
	}
}

func ResourceAuthenticationMiddleware() server.ResourceHandlerMiddleware {
	return func(next server.ResourceHandlerFunc) server.ResourceHandlerFunc {
		return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			if err := checkAuthentication(ctx); err != nil {
				return nil, err
			}
			return next(ctx, request)
		}
	}
}

func AuthenticatedPromptHandlerFunc(handler func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error)) func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		if err := checkAuthentication(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, request)
	}
}
