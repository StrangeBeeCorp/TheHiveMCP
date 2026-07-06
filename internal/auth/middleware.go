// Package auth provides fail-closed authentication middleware wrapping MCP
// tool, resource, and prompt handlers.
package auth

import (
	"context"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// ErrAuthenticationNotValidated is returned when a request reaches a handler
// without the positive authentication marker set by the transport context
// function.
var ErrAuthenticationNotValidated = errors.New("TheHive authentication failed: credentials were not validated")

// checkAuthentication enforces fail-closed authentication: a request is only
// allowed when the transport context function recorded a successful
// validation (types.AuthValidatedCtxKey) and no authentication error is
// present. A descriptive error from the context takes precedence over the
// generic missing-marker error.
func checkAuthentication(ctx context.Context) error {
	authError, ok := ctx.Value(types.AuthErrorCtxKey).(error)
	if ok && authError != nil {
		return authError
	}

	if validated, ok := ctx.Value(types.AuthValidatedCtxKey).(bool); !ok || !validated {
		return ErrAuthenticationNotValidated
	}

	return nil
}

// AuthenticationMiddleware returns a middleware function that requires a
// successful authentication marker before allowing tool operations to proceed
func AuthenticationMiddleware() server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			err := checkAuthentication(ctx)
			if err != nil {
				return nil, err
			}

			return next(ctx, request)
		}
	}
}

// ResourceAuthenticationMiddleware returns a middleware function that requires a
// successful authentication marker before allowing resource operations to proceed
func ResourceAuthenticationMiddleware() server.ResourceHandlerMiddleware {
	return func(next server.ResourceHandlerFunc) server.ResourceHandlerFunc {
		return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			err := checkAuthentication(ctx)
			if err != nil {
				return nil, err
			}

			return next(ctx, request)
		}
	}
}

// AuthenticatedPromptHandlerFunc wraps a prompt handler with authentication checking
func AuthenticatedPromptHandlerFunc(handler func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error)) func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		err := checkAuthentication(ctx)
		if err != nil {
			return nil, err
		}

		return handler(ctx, request)
	}
}
