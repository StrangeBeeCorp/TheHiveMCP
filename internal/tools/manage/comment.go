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

func (t *ManageTool) handleComment(ctx context.Context, params *manageParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings").Result()
	}

	results := make([]map[string]interface{}, 0, len(params.EntityIDs))

	for _, entityID := range params.EntityIDs {
		result, err := t.addComment(ctx, hiveClient, params.EntityType, entityID, params.Comment)
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
		"operation":  "comment",
		"entityType": params.EntityType,
		"results":    results,
	}), nil
}

func (t *ManageTool) addComment(ctx context.Context, client *thehive.APIClient, entityType, entityID, comment string) (interface{}, error) {
	switch entityType {
	case types.EntityTypeCase:
		inputComment := thehive.InputComment{
			Message: comment,
		}
		result, resp, err := client.CommentAPI.CreateCommentInCase(ctx, entityID).InputComment(inputComment).Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to add comment to case %s: %w. Check that the case exists and you have permissions to add comments. API response: %v", entityID, err, resp)
		}
		return result, nil

	case types.EntityTypeTask:
		inputTaskLog := thehive.InputCreateLog{
			Message: comment,
		}
		result, resp, err := client.TaskLogAPI.CreateTaskLog(ctx, entityID).InputCreateLog(inputTaskLog).Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to add task log to task %s: %w. Check that the task exists and you have permissions to add task logs. API response: %v", entityID, err, resp)
		}
		return result, nil

	default:
		return nil, fmt.Errorf("comments not supported for entity type '%s'. Comments are only supported on 'case' (adds comment) and 'task' (adds task log)", entityType)
	}
}
