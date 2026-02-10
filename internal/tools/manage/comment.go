package manage

import (
	"context"
	"fmt"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func (t *ManageTool) handleComment(ctx context.Context, params *ManageEntityParams) (ManageEntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	results := make([]SingleEntityCommentResult, 0, len(params.EntityIDs))

	for _, entityID := range params.EntityIDs {
		result := t.addComment(ctx, hiveClient, params.EntityType, entityID, params.Comment)
		results = append(results, result)
	}

	return ManageEntityResult{
		CommentResults: NewCommentEntityResult(params.EntityType, results),
	}, nil
}

func (t *ManageTool) addComment(ctx context.Context, client *thehive.APIClient, entityType, entityID, comment string) SingleEntityCommentResult {
	switch entityType {
	case types.EntityTypeCase:
		inputComment := thehive.InputComment{
			Message: comment,
		}
		result, resp, err := client.CommentAPI.CreateCommentInCase(ctx, entityID).InputComment(inputComment).Execute()
		if err != nil {
			return SingleEntityCommentResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to add comment to case").Cause(err).Hint("Check that the case exists and you have permissions to add comments.").API(resp).Error(),
			}
		}
		return SingleEntityCommentResult{
			EntityID: entityID,
			Result:   fmt.Sprintf("comment added with ID %s", result.GetUnderscoreId()),
		}

	case types.EntityTypeTask:
		inputTaskLog := thehive.InputCreateLog{
			Message: comment,
		}
		result, resp, err := client.TaskLogAPI.CreateTaskLog(ctx, entityID).InputCreateLog(inputTaskLog).Execute()
		if err != nil {
			return SingleEntityCommentResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to add task log to task").Cause(err).Hint("Check that the task exists and you have permissions to add task logs.").API(resp).Error(),
			}
		}
		return SingleEntityCommentResult{
			EntityID: entityID,
			Result:   fmt.Sprintf("task log added with ID %s", result.GetUnderscoreId()),
		}

	default:
		return SingleEntityCommentResult{
			EntityID: entityID,
			Error:    tools.NewToolError(fmt.Sprintf("comments not supported for entity type '%s'. Comments are only supported on 'case' (adds comment) and 'task' (adds task log)", entityType)).Error(),
		}
	}
}
