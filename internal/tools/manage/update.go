package manage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func (t *Tool) handleUpdate(ctx context.Context, params *EntityParams) (EntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	results := make([]SingleEntityUpdateResult, 0, len(params.EntityIDs))

	for _, entityID := range params.EntityIDs {
		result := t.updateEntity(ctx, hiveClient, params.EntityType, entityID, params.TargetID, params.EntityData)
		results = append(results, result)
	}

	return EntityResult{
		UpdateResults: NewUpdateEntityResult(params.EntityType, results),
	}, nil
}

func (t *Tool) updateEntity(ctx context.Context, client *thehive.APIClient, entityType, entityID, targetID string, data map[string]any) SingleEntityUpdateResult {
	// Convert ISO date strings to timestamps before marshaling
	data = utils.TranslateDatesToTimestamps(data)

	// Convert map to update structure
	jsonData, err := json.Marshal(data)
	if err != nil {
		return SingleEntityUpdateResult{
			EntityID: entityID,
			Error:    tools.NewToolError("Failed to marshal update data").Cause(err).Hint("Check that entity-data contains valid JSON fields for updating").ToMap(),
		}
	}

	switch entityType {
	case types.EntityTypeAlert:
		var input thehive.InputUpdateAlert
		if res, ok := unmarshalUpdate(entityID, jsonData, &input, types.EntityTypeAlert); !ok {
			return res
		}

		resp, err := client.AlertAPI.UpdateAlert(ctx, entityID).InputUpdateAlert(input).Execute()
		defer closeResponse(resp)

		return updateResult(entityID, err, "failed to update alert",
			fmt.Sprintf("Check that the alert %s exists and you have permissions. API response: %s", entityID, utils.DescribeHTTPResponse(resp)))

	case types.EntityTypeCase:
		var input thehive.InputUpdateCase
		if res, ok := unmarshalUpdate(entityID, jsonData, &input, types.EntityTypeCase); !ok {
			return res
		}

		resp, err := client.CaseAPI.UpdateCase(ctx, entityID).InputUpdateCase(input).Execute()
		defer closeResponse(resp)

		return updateResult(entityID, err, "failed to update case",
			fmt.Sprintf("Check that the case %s exists and you have permissions. API response: %s", entityID, utils.DescribeHTTPResponse(resp)))

	case types.EntityTypeTask:
		var input thehive.InputUpdateTask
		if res, ok := unmarshalUpdate(entityID, jsonData, &input, types.EntityTypeTask); !ok {
			return res
		}

		resp, err := client.TaskAPI.UpdateTask(ctx, entityID).InputUpdateTask(input).Execute()
		defer closeResponse(resp)

		return updateResult(entityID, err, "failed to update task",
			fmt.Sprintf("Check that the task %s exists and you have permissions. API response: %s", entityID, utils.DescribeHTTPResponse(resp)))

	case types.EntityTypeObservable:
		var input thehive.InputUpdateObservable
		if res, ok := unmarshalUpdate(entityID, jsonData, &input, types.EntityTypeObservable); !ok {
			return res
		}

		resp, err := client.ObservableAPI.UpdateObservable(ctx, entityID).InputUpdateObservable(input).Execute()
		defer closeResponse(resp)

		return updateResult(entityID, err, "failed to update observable",
			fmt.Sprintf("Check that the observable %s exists and you have permissions. API response: %s", entityID, utils.DescribeHTTPResponse(resp)))

	case types.EntityTypeProcedure:
		var input thehive.InputUpdateProcedure
		if res, ok := unmarshalUpdate(entityID, jsonData, &input, types.EntityTypeProcedure); !ok {
			return res
		}

		resp, err := client.TTPAPI.UpdateProcedure(ctx, entityID).InputUpdateProcedure(input).Execute()
		defer closeResponse(resp)

		return updateResult(entityID, err, "failed to update procedure",
			fmt.Sprintf("Check that the procedure %s exists and you have permissions. API response: %s", entityID, utils.DescribeHTTPResponse(resp)))

	case types.EntityTypeCaseTemplate:
		var input thehive.InputUpdateCaseTemplate
		if res, ok := unmarshalUpdate(entityID, jsonData, &input, types.EntityTypeCaseTemplate); !ok {
			return res
		}

		resp, err := client.CaseTemplateAPI.UpdateCaseTemplate(ctx, entityID).InputUpdateCaseTemplate(input).Execute()
		defer closeResponse(resp)

		return updateResult(entityID, err, "failed to update case template",
			fmt.Sprintf("Check that the case template %s exists and you have permissions. API response: %s", entityID, utils.DescribeHTTPResponse(resp)))

	case types.EntityTypePage:
		return t.updatePage(ctx, client, entityID, targetID, jsonData)

	default:
		return SingleEntityUpdateResult{
			EntityID: entityID,
			Error:    tools.NewToolError("unsupported entity type for update").Hint("Entity type: " + entityType).ToMap(),
		}
	}
}

// updatePage updates a page either standalone or from within its parent case.
func (t *Tool) updatePage(ctx context.Context, client *thehive.APIClient, entityID, targetID string, jsonData []byte) SingleEntityUpdateResult {
	var input thehive.InputUpdatePage
	if res, ok := unmarshalUpdate(entityID, jsonData, &input, types.EntityTypePage); !ok {
		return res
	}

	if targetID != "" {
		resp, err := client.PageAPI.UpdateAPageInACase(ctx, targetID, entityID).InputUpdatePage(input).Execute()
		defer closeResponse(resp)

		return updateResult(entityID, err, "failed to update page in case",
			fmt.Sprintf("Check that the page %s and case %s exist and you have permissions.", entityID, targetID))
	}

	resp, err := client.PageAPI.UpdateAPage(ctx, entityID).InputUpdatePage(input).Execute()
	defer closeResponse(resp)

	return updateResult(entityID, err, "failed to update page",
		fmt.Sprintf("Check that the page %s exists and you have permissions. For case-attached pages, provide the parent case ID in target-id.", entityID))
}

// unmarshalUpdate decodes jsonData into target. ok is false when decoding fails,
// in which case res carries the tool error to return.
func unmarshalUpdate(entityID string, jsonData []byte, target any, entityType string) (res SingleEntityUpdateResult, ok bool) {
	err := json.Unmarshal(jsonData, target)
	if err != nil {
		return SingleEntityUpdateResult{
			EntityID: entityID,
			Error: tools.NewToolErrorf("failed to unmarshal %s update data", entityType).Cause(err).
				Hintf("Use get-resource 'hive://schema/%s/update' to see updatable fields", entityType).ToMap(),
		}, false
	}

	return SingleEntityUpdateResult{}, true
}

// updateResult builds the per-entity update result from the API call outcome.
// The caller owns closing the response.
func updateResult(entityID string, err error, errMsg, errHint string) SingleEntityUpdateResult {
	if err != nil {
		return SingleEntityUpdateResult{
			EntityID: entityID,
			Error:    tools.NewToolError(errMsg).Cause(err).Hint(errHint).ToMap(),
		}
	}

	return SingleEntityUpdateResult{
		EntityID: entityID,
		Result:   resultUpdated,
	}
}
