package manage

import (
	"context"
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ManageTool) handleDelete(ctx context.Context, params *manageParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings").Result()
	}

	results := make([]map[string]interface{}, 0, len(params.EntityIDs))

	for _, entityID := range params.EntityIDs {
		err := t.deleteEntity(ctx, hiveClient, params.EntityType, entityID)
		if err != nil {
			results = append(results, map[string]interface{}{
				"id":    entityID,
				"error": err.Error(),
			})
		} else {
			results = append(results, map[string]interface{}{
				"id":      entityID,
				"deleted": true,
			})
		}
	}

	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":  "delete",
		"entityType": params.EntityType,
		"results":    results,
	}), nil
}

func (t *ManageTool) deleteEntity(ctx context.Context, client *thehive.APIClient, entityType, entityID string) error {
	switch entityType {
	case types.EntityTypeAlert:
		resp, err := client.AlertAPI.DeleteAlert(ctx, entityID).Execute()
		if err != nil {
			return fmt.Errorf("failed to delete alert %s: %w. Check that the alert exists and you have permissions. This operation is irreversible. API response: %v", entityID, err, resp)
		}
		return nil

	case types.EntityTypeCase:
		resp, err := client.CaseAPI.DeleteCase(ctx, entityID).Execute()
		if err != nil {
			return fmt.Errorf("failed to delete case %s: %w. Check that the case exists and you have permissions. This operation is irreversible. API response: %v", entityID, err, resp)
		}
		return nil

	case types.EntityTypeTask:
		resp, err := client.TaskAPI.DeleteTask(ctx, entityID).Execute()
		if err != nil {
			return fmt.Errorf("failed to delete task %s: %w. Check that the task exists and you have permissions. This operation is irreversible. API response: %v", entityID, err, resp)
		}
		return nil

	case types.EntityTypeObservable:
		resp, err := client.ObservableAPI.DeleteObservable(ctx, entityID).Execute()
		if err != nil {
			return fmt.Errorf("failed to delete observable %s: %w. Check that the observable exists and you have permissions. This operation is irreversible. API response: %v", entityID, err, resp)
		}
		return nil

	default:
		return fmt.Errorf("unsupported entity type for delete: %s", entityType)
	}
}
