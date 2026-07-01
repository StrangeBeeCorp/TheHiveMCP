package tools

import (
	"context"
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// WithValidation runs ValidateParams then ValidatePermissions before Handle, then date-processes the result.
func WithValidation[TParams, TResult any](tool Tool[TParams, TResult]) server.ToolHandlerFunc {
	wrapUntrusted := false
	if u, ok := any(tool).(UntrustedDataSource); ok {
		wrapUntrusted = u.HasUntrustedData()
	}

	businessHandler := func(ctx context.Context, request mcp.CallToolRequest, params TParams) (TResult, error) {
		if err := tool.ValidateParams(&params); err != nil {
			var zero TResult
			return zero, err
		}

		if err := tool.ValidatePermissions(ctx, params); err != nil {
			var zero TResult
			return zero, err
		}

		return tool.Handle(ctx, request, params)
	}

	return NewDateAwareHandler(businessHandler, wrapUntrusted)
}

// NewDateAwareHandler date-processes the result; when wrapUntrusted is true, user-generated fields are wrapped in boundary tags.
func NewDateAwareHandler[TParams, TResult any](handler func(ctx context.Context, req mcp.CallToolRequest, args TParams) (TResult, error), wrapUntrusted bool) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params TParams
		if err := req.BindArguments(&params); err != nil {
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
