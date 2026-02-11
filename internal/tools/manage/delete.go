package manage

import (
	"context"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func (t *ManageTool) handleDelete(ctx context.Context, params *ManageEntityParams) (ManageEntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	results := make([]SingleEntityDeleteResult, 0, len(params.EntityIDs))

	for _, entityID := range params.EntityIDs {
		result := t.deleteEntity(ctx, hiveClient, params.EntityType, entityID)
		results = append(results, result)
	}

	return ManageEntityResult{
		DeleteResults: NewDeleteEntityResult(params.EntityType, results),
	}, nil
}

func (t *ManageTool) deleteEntity(ctx context.Context, client *thehive.APIClient, entityType, entityID string) SingleEntityDeleteResult {
	switch entityType {
	case types.EntityTypeAlert:
		resp, err := client.AlertAPI.DeleteAlert(ctx, entityID).Execute()
		if err != nil {
			return SingleEntityDeleteResult{
				EntityID: entityID,
				Error: tools.NewToolErrorf(
					"failed to delete alert %s: %v. Check that the alert exists and you have permissions. This operation is irreversible.", entityID, err).API(resp).ToMap(),
			}
		}
		return SingleEntityDeleteResult{
			EntityID: entityID,
			Deleted:  true,
		}

	case types.EntityTypeCase:
		resp, err := client.CaseAPI.DeleteCase(ctx, entityID).Execute()
		if err != nil {
			return SingleEntityDeleteResult{
				EntityID: entityID,
				Error:    tools.NewToolErrorf("failed to delete case %s: %v. Check that the case exists and you have permissions. This operation is irreversible.", entityID, err).API(resp).ToMap(),
			}
		}
		return SingleEntityDeleteResult{
			EntityID: entityID,
			Deleted:  true,
		}

	case types.EntityTypeTask:
		resp, err := client.TaskAPI.DeleteTask(ctx, entityID).Execute()
		if err != nil {
			return SingleEntityDeleteResult{
				EntityID: entityID,
				Error:    tools.NewToolErrorf("failed to delete task %s: %v. Check that the task exists and you have permissions. This operation is irreversible.", entityID, err).API(resp).ToMap(),
			}
		}
		return SingleEntityDeleteResult{
			EntityID: entityID,
			Deleted:  true,
		}

	case types.EntityTypeObservable:
		resp, err := client.ObservableAPI.DeleteObservable(ctx, entityID).Execute()
		if err != nil {
			return SingleEntityDeleteResult{
				EntityID: entityID,
				Error:    tools.NewToolErrorf("failed to delete observable %s: %v. Check that the observable exists and you have permissions. This operation is irreversible.", entityID, err).API(resp).ToMap(),
			}
		}
		return SingleEntityDeleteResult{
			EntityID: entityID,
			Deleted:  true,
		}

	default:
		return SingleEntityDeleteResult{
			EntityID: entityID,
			Error:    tools.NewToolErrorf("unsupported entity type for delete: %s", entityType).ToMap(),
		}
	}
}
