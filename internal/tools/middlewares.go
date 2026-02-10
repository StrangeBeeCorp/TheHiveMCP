package tools

import (
	"context"
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// WithValidation wraps a tool's Handler with validation and date processing
func WithValidation[TParams, TResult any](tool Tool[TParams, TResult]) server.ToolHandlerFunc {
	businessHandler := func(ctx context.Context, request mcp.CallToolRequest, params TParams) (TResult, error) {
		// 1. Validate parameters and apply defaults
		if err := tool.ValidateParams(&params); err != nil {
			var zero TResult
			return zero, err
		}

		// 2. Validate permissions
		if err := tool.ValidatePermissions(ctx, params); err != nil {
			var zero TResult
			return zero, err
		}

		// 3. Call the main handler
		return tool.Handle(ctx, request, params)
	}

	return NewDateAwareHandler(businessHandler)
}

// NewDateAwareHandler creates a handler with automatic date processing for any result type
func NewDateAwareHandler[TParams, TResult any](handler func(ctx context.Context, req mcp.CallToolRequest, args TParams) (TResult, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// 1. Extract parameters
		var params TParams
		if err := req.BindArguments(&params); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		// 2. Call handler
		result, err := handler(ctx, req, params)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// 3. Process dates in result (converts any type to interface{} then processes)
		processedResult, err := utils.ProcessDatesRecursive(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to process dates: %v", err)), nil
		}

		// 4. Return as JSON
		toolResult := utils.NewToolResultJSONUnescaped(processedResult)
		return toolResult, nil
	}
}
