package manage

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

const (
	// hintValidJSONFields is the error hint shown when entity-data fails to marshal.
	hintValidJSONFields = "Check that entity-data contains valid JSON fields"
	// hintRequiredFields is the error hint shown when a create call is rejected.
	hintRequiredFields = "Check required fields and permissions"
)

func (t *Tool) handleCreate(ctx context.Context, params *EntityParams) (EntityResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to get TheHive client").Cause(err).
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
	case types.EntityTypeProcedure:
		return t.createProcedure(ctx, hiveClient, processedData, params.EntityIDs[0])
	case types.EntityTypeCaseTemplate:
		return t.createCaseTemplate(ctx, hiveClient, processedData)
	case types.EntityTypePage:
		var parentID string
		if len(params.EntityIDs) > 0 {
			parentID = params.EntityIDs[0]
		}

		return t.createPage(ctx, hiveClient, processedData, parentID)
	default:
		return EntityResult{}, tools.NewToolErrorf("unsupported entity type for create: %s", params.EntityType)
	}
}

func (t *Tool) createAlert(ctx context.Context, client *thehive.APIClient, data map[string]any) (EntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to marshal alert data").Cause(err).
			Hint(hintValidJSONFields).
			Schema("alert", "create")
	}

	var inputAlert thehive.InputCreateAlert

	err = json.Unmarshal(jsonData, &inputAlert)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to unmarshal alert data").Cause(err).
			Hint("Ensure entity-data fields match the alert schema").
			Schema("alert", "create")
	}

	alert, resp, err := client.AlertAPI.CreateAlert(ctx).InputCreateAlert(inputAlert).Execute()
	defer closeResponse(resp)

	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to create alert").Cause(err).
			Hint(hintRequiredFields).API(resp)
	}

	return EntityResult{
		CreateAlertResult: NewCreateAlertResult(alert),
	}, nil
}

func (t *Tool) createCase(ctx context.Context, client *thehive.APIClient, data map[string]any) (EntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to marshal case data").Cause(err).
			Hint(hintValidJSONFields).
			Schema("case", "create")
	}

	var inputCase thehive.InputCreateCase

	err = json.Unmarshal(jsonData, &inputCase)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to unmarshal case data").Cause(err).
			Hint("Ensure entity-data fields match the case schema").
			Schema("case", "create")
	}

	result, resp, err := client.CaseAPI.CreateCase(ctx).InputCreateCase(inputCase).Execute()
	defer closeResponse(resp)

	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to create case").Cause(err).
			Hint(hintRequiredFields).API(resp)
	}

	// For create operations, return the single entity, not an array
	return EntityResult{
		CreateCaseResult: NewCreateCaseResult(result),
	}, nil
}

func (t *Tool) createTask(ctx context.Context, client *thehive.APIClient, data map[string]any, parentID string) (EntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to marshal task data").Cause(err).
			Hint(hintValidJSONFields).
			Schema("task", "create")
	}

	var inputTask thehive.InputCreateTask

	err = json.Unmarshal(jsonData, &inputTask)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to unmarshal task data").Cause(err).
			Hint("Ensure entity-data fields match the task schema").
			Schema("task", "create")
	}

	result, resp, err := client.TaskAPI.CreateTaskInCase(ctx, parentID).InputCreateTask(inputTask).Execute()
	defer closeResponse(resp)

	if err != nil {
		return EntityResult{}, tools.NewToolErrorf("failed to create task in case %s", parentID).Cause(err).
			Hint("Check that the case exists and you have permissions").API(resp)
	}
	// For create operations, return the single entity, not an array
	return EntityResult{
		CreateTaskResult: NewCreateTaskResult(result),
	}, nil
}

func (t *Tool) createObservable(ctx context.Context, client *thehive.APIClient, data map[string]any, parentID string) (EntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to marshal observable data").Cause(err).
			Hint(hintValidJSONFields).
			Schema("observable", "create")
	}

	var inputObservable thehive.InputCreateObservable

	err = json.Unmarshal(jsonData, &inputObservable)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to unmarshal observable data").Cause(err).
			Hint("Ensure entity-data fields match the observable schema").
			Schema("observable", "create")
	}

	// Try to create in case first, then alert if that fails
	var result []thehive.OutputObservable

	// First attempt with case
	caseResult, caseResp, caseErr := client.ObservableAPI.CreateObservableInCase(ctx, parentID).InputCreateObservable(inputObservable).Execute()
	defer closeResponse(caseResp)

	if caseErr != nil {
		// If case creation fails, try alert
		alertResult, alertResp, alertErr := client.ObservableAPI.CreateObservableInAlert(ctx, parentID).InputCreateObservable(inputObservable).Execute()
		defer closeResponse(alertResp)

		if alertErr != nil {
			return EntityResult{}, tools.NewToolError("failed to create observable").Cause(alertErr).
				Hint("Check that the target case/alert exists and you have permissions")
		}

		result = alertResult
	} else {
		result = caseResult
	}

	return EntityResult{
		CreateObservableResult: NewCreateObservableResult(result),
	}, nil
}

func (t *Tool) createProcedure(ctx context.Context, client *thehive.APIClient, data map[string]any, parentID string) (EntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to marshal procedure data").Cause(err).
			Hint(hintValidJSONFields).
			Schema("procedure", "create")
	}

	var inputProcedure thehive.InputProcedure

	err = json.Unmarshal(jsonData, &inputProcedure)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to unmarshal procedure data").Cause(err).
			Hint("Ensure entity-data fields match the procedure schema").
			Schema("procedure", "create")
	}

	// First attempt with case
	caseResult, caseResp, caseErr := client.TTPAPI.CreateProcedureForCase(ctx, parentID).InputProcedure(inputProcedure).Execute()
	defer closeResponse(caseResp)

	if caseErr != nil {
		// If case creation fails, try alert
		alertResult, alertResp, alertErr := client.TTPAPI.CreateProcedureForAlert(ctx, parentID).InputProcedure(inputProcedure).Execute()
		defer closeResponse(alertResp)

		if alertErr != nil {
			return EntityResult{}, tools.NewToolError("failed to create procedure").Cause(alertErr).
				Hint("Check that the target case/alert exists and you have permissions")
		}

		return EntityResult{
			CreateProcedureResult: NewCreateProcedureResult(alertResult),
		}, nil
	}

	return EntityResult{
		CreateProcedureResult: NewCreateProcedureResult(caseResult),
	}, nil
}

func (t *Tool) createCaseTemplate(ctx context.Context, client *thehive.APIClient, data map[string]any) (EntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to marshal case template data").Cause(err).
			Hint(hintValidJSONFields).
			Schema("case-template", "create")
	}

	var inputCaseTemplate thehive.InputCreateCaseTemplate

	err = json.Unmarshal(jsonData, &inputCaseTemplate)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to unmarshal case template data").Cause(err).
			Hint("Ensure entity-data fields match the case template schema").
			Schema("case-template", "create")
	}

	result, resp, err := client.CaseTemplateAPI.CreateCaseTemplate(ctx).InputCreateCaseTemplate(inputCaseTemplate).Execute()
	defer closeResponse(resp)

	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to create case template").Cause(err).
			Hint("Check required fields (name) and permissions").API(resp)
	}

	return EntityResult{
		CreateCaseTemplateResult: NewCreateCaseTemplateResult(result),
	}, nil
}

func (t *Tool) createPage(ctx context.Context, client *thehive.APIClient, data map[string]any, parentID string) (EntityResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to marshal page data").Cause(err).
			Hint(hintValidJSONFields).
			Schema("page", "create")
	}

	var inputPage thehive.InputCreatePage

	err = json.Unmarshal(jsonData, &inputPage)
	if err != nil {
		return EntityResult{}, tools.NewToolError("failed to unmarshal page data").Cause(err).
			Hint("Ensure entity-data fields match the page schema").
			Schema("page", "create")
	}

	var (
		result *thehive.OutputPage
		resp   *http.Response
	)

	if parentID != "" {
		result, resp, err = client.PageAPI.CreateAPageInACase(ctx, parentID).InputCreatePage(inputPage).Execute()
		defer closeResponse(resp)

		if err != nil {
			return EntityResult{}, tools.NewToolErrorf("failed to create page in case %s", parentID).Cause(err).
				Hint("Check that the case exists and you have permissions").API(resp)
		}
	} else {
		result, resp, err = client.PageAPI.CreateAPage(ctx).InputCreatePage(inputPage).Execute()
		defer closeResponse(resp)

		if err != nil {
			return EntityResult{}, tools.NewToolError("failed to create standalone page").Cause(err).
				Hint(hintRequiredFields).API(resp)
		}
	}

	return EntityResult{
		CreatePageResult: NewCreatePageResult(result),
	}, nil
}
