package manage

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
)

// Handle dispatches the manage-entities request to the handler for the requested operation.
func (t *Tool) Handle(ctx context.Context, _ mcp.CallToolRequest, params EntityParams) (EntityResult, error) {
	switch params.Operation {
	case OperationCreate:
		return t.handleCreate(ctx, &params)
	case OperationUpdate:
		return t.handleUpdate(ctx, &params)
	case OperationDelete:
		return t.handleDelete(ctx, &params)
	case OperationComment:
		return t.handleComment(ctx, &params)
	case OperationPromote:
		return t.handlePromote(ctx, &params)
	case OperationMerge:
		return t.handleMerge(ctx, &params)
	case OperationApplyTemplate:
		return t.handleApplyTemplate(ctx, &params)
	default:
		return EntityResult{}, tools.NewToolErrorf("unsupported operation: %s", params.Operation)
	}
}
