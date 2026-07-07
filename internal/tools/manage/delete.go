package manage

import (
	"context"
	"net/http"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func (t *Tool) handleDelete(ctx context.Context, params *EntityParams) (EntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	results := make([]SingleEntityDeleteResult, 0, len(params.EntityIDs))

	for _, entityID := range params.EntityIDs {
		result := t.deleteEntity(ctx, hiveClient, params.EntityType, entityID, params.TargetID)
		results = append(results, result)
	}

	return EntityResult{
		DeleteResults: NewDeleteEntityResult(params.EntityType, results),
	}, nil
}

func (t *Tool) deleteEntity(ctx context.Context, client *thehive.APIClient, entityType, entityID, targetID string) SingleEntityDeleteResult {
	switch entityType {
	case types.EntityTypeAlert:
		resp, err := client.AlertAPI.DeleteAlert(ctx, entityID).Execute()
		defer closeResponse(resp)

		return deleteResult(entityID, resp, err,
			"failed to delete alert %s: %v. Check that the alert exists and you have permissions. This operation is irreversible.", entityID, err)

	case types.EntityTypeCase:
		resp, err := client.CaseAPI.DeleteCase(ctx, entityID).Execute()
		defer closeResponse(resp)

		return deleteResult(entityID, resp, err,
			"failed to delete case %s: %v. Check that the case exists and you have permissions. This operation is irreversible.", entityID, err)

	case types.EntityTypeTask:
		resp, err := client.TaskAPI.DeleteTask(ctx, entityID).Execute()
		defer closeResponse(resp)

		return deleteResult(entityID, resp, err,
			"failed to delete task %s: %v. Check that the task exists and you have permissions. This operation is irreversible.", entityID, err)

	case types.EntityTypeObservable:
		resp, err := client.ObservableAPI.DeleteObservable(ctx, entityID).Execute()
		defer closeResponse(resp)

		return deleteResult(entityID, resp, err,
			"failed to delete observable %s: %v. Check that the observable exists and you have permissions. This operation is irreversible.", entityID, err)

	case types.EntityTypeProcedure:
		resp, err := client.TTPAPI.DeleteProcedure(ctx, entityID).Execute()
		defer closeResponse(resp)

		return deleteResult(entityID, resp, err,
			"failed to delete procedure %s: %v. Check that the procedure exists and you have permissions. This operation is irreversible.", entityID, err)

	case types.EntityTypeCaseTemplate:
		resp, err := client.CaseTemplateAPI.DeleteCaseTemplate(ctx, entityID).Execute()
		defer closeResponse(resp)

		return deleteResult(entityID, resp, err,
			"failed to delete case template %s: %v. Check that the case template exists and you have permissions. This operation is irreversible.", entityID, err)

	case types.EntityTypePage:
		return t.deletePage(ctx, client, entityID, targetID)

	default:
		return SingleEntityDeleteResult{
			EntityID: entityID,
			Error:    tools.NewToolErrorf("unsupported entity type for delete: %s", entityType).ToMap(),
		}
	}
}

// deletePage deletes a page either standalone or from within its parent case.
func (t *Tool) deletePage(ctx context.Context, client *thehive.APIClient, entityID, targetID string) SingleEntityDeleteResult {
	if targetID != "" {
		resp, err := client.PageAPI.DeleteAPageInACase(ctx, targetID, entityID).Execute()
		defer closeResponse(resp)

		return deleteResult(entityID, resp, err,
			"failed to delete page %s in case %s: %v. Check that the page and case exist and you have permissions. This operation is irreversible.", entityID, targetID, err)
	}

	resp, err := client.PageAPI.DeleteAPage(ctx, entityID).Execute()
	defer closeResponse(resp)

	return deleteResult(entityID, resp, err,
		"failed to delete page %s: %v. Check that the page exists and you have permissions. For case-attached pages, provide the parent case ID in target-id. This operation is irreversible.", entityID, err)
}

// deleteResult builds the per-entity delete result, formatting errFormat/errArgs
// into a tool error when the API call failed and reporting success otherwise.
// The caller owns closing resp.
func deleteResult(entityID string, resp *http.Response, err error, errFormat string, errArgs ...any) SingleEntityDeleteResult {
	if err != nil {
		return SingleEntityDeleteResult{
			EntityID: entityID,
			Error:    tools.NewToolErrorf(errFormat, errArgs...).API(resp).ToMap(),
		}
	}

	return SingleEntityDeleteResult{
		EntityID: entityID,
		Deleted:  true,
	}
}
