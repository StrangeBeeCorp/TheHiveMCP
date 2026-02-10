package manage

import (
	"context"
	"encoding/json"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

func (t *ManageTool) handleCreate(ctx context.Context, params *ManageEntityParams) (ManageEntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings")
	}

	processedData := utils.TranslateDatesToTimestamps(params.EntityData)

	switch params.EntityType {
	case types.EntityTypeAlert:
		return t.createAlert(ctx, hiveClient, processedData)
	case types.EntityTypeCase:
		return t.createCase(ctx, hiveClient, processedData)
	case types.EntityTypeTask:
		return t.createTask(ctx, hiveClient, processedData, params.EntityIDs[0])
	case types.EntityTypeObservable:
		return t.createObservable(ctx, hiveClient, processedData, params.EntityIDs[0])
	default:
		return ManageEntityResult{}, tools.NewToolErrorf("unsupported entity type for create: %s", params.EntityType)
	}
}

func (t *ManageTool) createAlert(ctx context.Context, client *thehive.APIClient, data map[string]interface{}) (ManageEntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to marshal alert data").Cause(err).
			Hint("Check that entity-data contains valid JSON fields").
			Schema("alert", "create")
	}

	var inputAlert thehive.InputCreateAlert
	if err := json.Unmarshal(jsonData, &inputAlert); err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to unmarshal alert data").Cause(err).
			Hint("Ensure entity-data fields match the alert schema").
			Schema("alert", "create")
	}

	alert, resp, err := client.AlertAPI.CreateAlert(ctx).InputCreateAlert(inputAlert).Execute()
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to create alert").Cause(err).
			Hint("Check required fields and permissions").API(resp)
	}

	return ManageEntityResult{
		CreateAlertResult: NewCreateAlertResult(alert),
	}, nil
}

func (t *ManageTool) createCase(ctx context.Context, client *thehive.APIClient, data map[string]interface{}) (ManageEntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to marshal case data").Cause(err).
			Hint("Check that entity-data contains valid JSON fields").
			Schema("case", "create")
	}

	var inputCase thehive.InputCreateCase
	if err := json.Unmarshal(jsonData, &inputCase); err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to unmarshal case data").Cause(err).
			Hint("Ensure entity-data fields match the case schema").
			Schema("case", "create")
	}

	result, resp, err := client.CaseAPI.CreateCase(ctx).InputCreateCase(inputCase).Execute()
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to create case").Cause(err).
			Hint("Check required fields and permissions").API(resp)
	}

	// For create operations, return the single entity, not an array
	return ManageEntityResult{
		CreateCaseResult: NewCreateCaseResult(result),
	}, nil
}

func (t *ManageTool) createTask(ctx context.Context, client *thehive.APIClient, data map[string]interface{}, parentID string) (ManageEntityResult, error) {

	jsonData, err := json.Marshal(data)
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to marshal task data").Cause(err).
			Hint("Check that entity-data contains valid JSON fields").
			Schema("task", "create")
	}

	var inputTask thehive.InputCreateTask
	if err := json.Unmarshal(jsonData, &inputTask); err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to unmarshal task data").Cause(err).
			Hint("Ensure entity-data fields match the task schema").
			Schema("task", "create")
	}

	result, resp, err := client.TaskAPI.CreateTaskInCase(ctx, parentID).InputCreateTask(inputTask).Execute()
	if err != nil {
		return ManageEntityResult{}, tools.NewToolErrorf("failed to create task in case %s", parentID).Cause(err).
			Hint("Check that the case exists and you have permissions").API(resp)
	}
	// For create operations, return the single entity, not an array
	return ManageEntityResult{
		CreateTaskResult: NewCreateTaskResult(result),
	}, nil
}

func (t *ManageTool) createObservable(ctx context.Context, client *thehive.APIClient, data map[string]interface{}, parentID string) (ManageEntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to marshal observable data").Cause(err).
			Hint("Check that entity-data contains valid JSON fields").
			Schema("observable", "create")
	}

	var inputObservable thehive.InputCreateObservable
	if err := json.Unmarshal(jsonData, &inputObservable); err != nil {
		return ManageEntityResult{}, tools.NewToolError("failed to unmarshal observable data").Cause(err).
			Hint("Ensure entity-data fields match the observable schema").
			Schema("observable", "create")
	}

	// Try to create in case first, then alert if that fails
	var result []thehive.OutputObservable

	// First attempt with case
	caseResult, _, caseErr := client.ObservableAPI.CreateObservableInCase(ctx, parentID).InputCreateObservable(inputObservable).Execute()
	if caseErr != nil {
		// If case creation fails, try alert
		alertResult, _, alertErr := client.ObservableAPI.CreateObservableInAlert(ctx, parentID).InputCreateObservable(inputObservable).Execute()
		if alertErr != nil {
			return ManageEntityResult{}, tools.NewToolError("failed to create observable").Cause(alertErr).
				Hint("Check that the target case/alert exists and you have permissions")
		}
		result = alertResult
	} else {
		result = caseResult
	}

	return ManageEntityResult{
		CreateObservableResult: NewCreateObservableResult(result),
	}, nil

}
