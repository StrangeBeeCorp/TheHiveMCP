package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// WithValidation runs ValidateParams then ValidatePermissions before Handle, then date-processes the result.
func WithValidation[TParams, TResult any](tool Tool[TParams, TResult]) server.ToolHandlerFunc {
	wrapUntrusted := false
	if u, ok := any(tool).(UntrustedDataReporter); ok {
		wrapUntrusted = u.HasUntrustedData()
	}

	businessHandler := func(ctx context.Context, request mcp.CallToolRequest, params TParams) (TResult, error) {
		// Errors from ValidateParams/ValidatePermissions are already crafted as
		// user-facing messages (with hints and examples) surfaced verbatim to the
		// MCP client, so they are returned unwrapped by design.
		err := tool.ValidateParams(&params)
		if err != nil {
			var zero TResult
			return zero, err //nolint:wrapcheck // user-facing validation message, see above
		}

		err = tool.ValidatePermissions(ctx, params)
		if err != nil {
			var zero TResult
			return zero, err //nolint:wrapcheck // user-facing validation message, see above
		}

		return tool.Handle(ctx, request, params)
	}

	return NewDateAwareHandler(businessHandler, wrapUntrusted)
}

// NewDateAwareHandler date-processes the result; when wrapUntrusted is true, user-generated fields are wrapped in boundary tags.
func NewDateAwareHandler[TParams, TResult any](handler func(ctx context.Context, req mcp.CallToolRequest, args TParams) (TResult, error), wrapUntrusted bool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params TParams

		err := req.BindArguments(&params)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		result, err := handler(ctx, req, params)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		processedResult, err := utils.ProcessDatesRecursive(result, wrapUntrusted)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to process dates: %v", err)), nil
		}

		toolResult := utils.NewToolResultJSONUnescaped(processedResult)

		return toolResult, nil
	}
}
