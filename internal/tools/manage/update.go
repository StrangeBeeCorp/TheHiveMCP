package manage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ManageTool) handleUpdate(ctx context.Context, params *manageParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings").Result()
	}

	results := make([]map[string]interface{}, 0, len(params.EntityIDs))

	for _, entityID := range params.EntityIDs {
		result, err := t.updateEntity(ctx, hiveClient, params.EntityType, entityID, params.EntityData)
		if err != nil {
			results = append(results, map[string]interface{}{
				"id":    entityID,
				"error": err.Error(),
			})
		} else {
			results = append(results, map[string]interface{}{
				"id":     entityID,
				"result": result,
			})
		}
	}

	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":  "update",
		"entityType": params.EntityType,
		"results":    results,
	}), nil
}

func (t *ManageTool) updateEntity(ctx context.Context, client *thehive.APIClient, entityType, entityID string, data map[string]interface{}) (interface{}, error) {
	// Convert map to update structure
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal update data: %w. Check that entity-data contains valid JSON fields for updating", err)
	}

	switch entityType {
	case types.EntityTypeAlert:
		var inputAlert thehive.InputUpdateAlert
		if err := json.Unmarshal(jsonData, &inputAlert); err != nil {
			return nil, fmt.Errorf("failed to unmarshal alert update data: %w. Use get-resource 'hive://schema/alert/update' to see updatable fields", err)
		}
		resp, err := client.AlertAPI.UpdateAlert(ctx, entityID).InputUpdateAlert(inputAlert).Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to update alert %s: %w. Check that the alert exists and you have permissions. API response: %v", entityID, err, resp)
		}
		return "updated", nil

	case types.EntityTypeCase:
		var inputCase thehive.InputUpdateCase
		if err := json.Unmarshal(jsonData, &inputCase); err != nil {
			return nil, fmt.Errorf("failed to unmarshal case update data: %w. Use get-resource 'hive://schema/case/update' to see updatable fields", err)
		}
		resp, err := client.CaseAPI.UpdateCase(ctx, entityID).InputUpdateCase(inputCase).Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to update case %s: %w. Check that the case exists and you have permissions. API response: %v", entityID, err, resp)
		}
		return "updated", nil

	case types.EntityTypeTask:
		var inputTask thehive.InputUpdateTask
		if err := json.Unmarshal(jsonData, &inputTask); err != nil {
			return nil, fmt.Errorf("failed to unmarshal task update data: %w. Use get-resource 'hive://schema/task/update' to see updatable fields", err)
		}
		resp, err := client.TaskAPI.UpdateTask(ctx, entityID).InputUpdateTask(inputTask).Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to update task %s: %w. Check that the task exists and you have permissions. API response: %v", entityID, err, resp)
		}
		return "updated", nil

	case types.EntityTypeObservable:
		var inputObservable thehive.InputUpdateObservable
		if err := json.Unmarshal(jsonData, &inputObservable); err != nil {
			return nil, fmt.Errorf("failed to unmarshal observable update data: %w. Use get-resource 'hive://schema/observable/update' to see updatable fields", err)
		}
		resp, err := client.ObservableAPI.UpdateObservable(ctx, entityID).InputUpdateObservable(inputObservable).Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to update observable %s: %w. Check that the observable exists and you have permissions. API response: %v", entityID, err, resp)
		}
		return "updated", nil

	default:
		return nil, fmt.Errorf("unsupported entity type for update: %s", entityType)
	}
}
