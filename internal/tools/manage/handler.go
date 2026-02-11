package manage

import (
	"context"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ManageTool) Handle(ctx context.Context, request mcp.CallToolRequest, params ManageEntityParams) (ManageEntityResult, error) {

	switch params.Operation {
	case "create":
		return t.handleCreate(ctx, &params)
	case "update":
		return t.handleUpdate(ctx, &params)
	case "delete":
		return t.handleDelete(ctx, &params)
	case "comment":
		return t.handleComment(ctx, &params)
	case "promote":
		return t.handlePromote(ctx, &params)
	case "merge":
		return t.handleMerge(ctx, &params)
	default:
		return ManageEntityResult{}, tools.NewToolErrorf("unsupported operation: %s", params.Operation)
	}
}
