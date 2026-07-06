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
		var inputAlert thehive.InputUpdateAlert

		err = json.Unmarshal(jsonData, &inputAlert)
		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to unmarshal alert update data").Cause(err).Hint("Use get-resource 'hive://schema/alert/update' to see updatable fields").ToMap(),
			}
		}

		resp, err := client.AlertAPI.UpdateAlert(ctx, entityID).InputUpdateAlert(inputAlert).Execute()
		defer closeResponse(resp)

		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to update alert").Cause(err).Hint(fmt.Sprintf("Check that the alert %s exists and you have permissions. API response: %v", entityID, resp)).ToMap(),
			}
		}

		return SingleEntityUpdateResult{
			EntityID: entityID,
			Result:   resultUpdated,
		}

	case types.EntityTypeCase:
		var inputCase thehive.InputUpdateCase

		err = json.Unmarshal(jsonData, &inputCase)
		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to unmarshal case update data").Cause(err).Hint("Use get-resource 'hive://schema/case/update' to see updatable fields").ToMap(),
			}
		}

		resp, err := client.CaseAPI.UpdateCase(ctx, entityID).InputUpdateCase(inputCase).Execute()
		defer closeResponse(resp)

		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to update case").Cause(err).Hint(fmt.Sprintf("Check that the case %s exists and you have permissions. API response: %v", entityID, resp)).ToMap(),
			}
		}

		return SingleEntityUpdateResult{
			EntityID: entityID,
			Result:   resultUpdated,
		}

	case types.EntityTypeTask:
		var inputTask thehive.InputUpdateTask

		err = json.Unmarshal(jsonData, &inputTask)
		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to unmarshal task update data").Cause(err).Hint("Use get-resource 'hive://schema/task/update' to see updatable fields").ToMap(),
			}
		}

		resp, err := client.TaskAPI.UpdateTask(ctx, entityID).InputUpdateTask(inputTask).Execute()
		defer closeResponse(resp)

		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to update task").Cause(err).Hint(fmt.Sprintf("Check that the task %s exists and you have permissions. API response: %v", entityID, resp)).ToMap(),
			}
		}

		return SingleEntityUpdateResult{
			EntityID: entityID,
			Result:   resultUpdated,
		}

	case types.EntityTypeObservable:
		var inputObservable thehive.InputUpdateObservable

		err = json.Unmarshal(jsonData, &inputObservable)
		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to unmarshal observable update data").Cause(err).Hint("Use get-resource 'hive://schema/observable/update' to see updatable fields").ToMap(),
			}
		}

		resp, err := client.ObservableAPI.UpdateObservable(ctx, entityID).InputUpdateObservable(inputObservable).Execute()
		defer closeResponse(resp)

		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to update observable").Cause(err).Hint(fmt.Sprintf("Check that the observable %s exists and you have permissions. API response: %v", entityID, resp)).ToMap(),
			}
		}

		return SingleEntityUpdateResult{
			EntityID: entityID,
			Result:   resultUpdated,
		}

	case types.EntityTypeProcedure:
		var inputProcedure thehive.InputUpdateProcedure

		err = json.Unmarshal(jsonData, &inputProcedure)
		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to unmarshal procedure update data").Cause(err).Hint("Use get-resource 'hive://schema/procedure/update' to see updatable fields").ToMap(),
			}
		}

		resp, err := client.TTPAPI.UpdateProcedure(ctx, entityID).InputUpdateProcedure(inputProcedure).Execute()
		defer closeResponse(resp)

		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to update procedure").Cause(err).Hint(fmt.Sprintf("Check that the procedure %s exists and you have permissions. API response: %v", entityID, resp)).ToMap(),
			}
		}

		return SingleEntityUpdateResult{
			EntityID: entityID,
			Result:   resultUpdated,
		}

	case types.EntityTypeCaseTemplate:
		var inputCaseTemplate thehive.InputUpdateCaseTemplate

		err = json.Unmarshal(jsonData, &inputCaseTemplate)
		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to unmarshal case template update data").Cause(err).Hint("Use get-resource 'hive://schema/case-template/update' to see updatable fields").ToMap(),
			}
		}

		resp, err := client.CaseTemplateAPI.UpdateCaseTemplate(ctx, entityID).InputUpdateCaseTemplate(inputCaseTemplate).Execute()
		defer closeResponse(resp)

		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to update case template").Cause(err).Hint(fmt.Sprintf("Check that the case template %s exists and you have permissions. API response: %v", entityID, resp)).ToMap(),
			}
		}

		return SingleEntityUpdateResult{
			EntityID: entityID,
			Result:   resultUpdated,
		}

	case types.EntityTypePage:
		var inputPage thehive.InputUpdatePage

		err := json.Unmarshal(jsonData, &inputPage)
		if err != nil {
			return SingleEntityUpdateResult{
				EntityID: entityID,
				Error:    tools.NewToolError("failed to unmarshal page update data").Cause(err).Hint("Use get-resource 'hive://schema/page/update' to see updatable fields").ToMap(),
			}
		}

		if targetID != "" {
			resp, err := client.PageAPI.UpdateAPageInACase(ctx, targetID, entityID).InputUpdatePage(inputPage).Execute()
			defer closeResponse(resp)

			if err != nil {
				return SingleEntityUpdateResult{
					EntityID: entityID,
					Error:    tools.NewToolError("failed to update page in case").Cause(err).Hint(fmt.Sprintf("Check that the page %s and case %s exist and you have permissions. API response: %v", entityID, targetID, resp)).ToMap(),
				}
			}
		} else {
			resp, err := client.PageAPI.UpdateAPage(ctx, entityID).InputUpdatePage(inputPage).Execute()
			defer closeResponse(resp)

			if err != nil {
				return SingleEntityUpdateResult{
					EntityID: entityID,
					Error:    tools.NewToolError("failed to update page").Cause(err).Hint(fmt.Sprintf("Check that the page %s exists and you have permissions. For case-attached pages, provide the parent case ID in target-id. API response: %v", entityID, resp)).ToMap(),
				}
			}
		}

		return SingleEntityUpdateResult{
			EntityID: entityID,
			Result:   resultUpdated,
		}

	default:
		return SingleEntityUpdateResult{
			EntityID: entityID,
			Error:    tools.NewToolError("unsupported entity type for update").Hint("Entity type: " + entityType).ToMap(),
		}
	}
}
