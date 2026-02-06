package manage

import (
	"context"
	"encoding/json"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ManageTool) handleCreate(ctx context.Context, params *manageParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get TheHive client").Cause(err).
			Hint("Check your authentication and connection settings").Result()
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
		return tools.NewToolErrorf("unsupported entity type for create: %s", params.EntityType).Result()
	}
}

func (t *ManageTool) createAlert(ctx context.Context, client *thehive.APIClient, data map[string]interface{}) (*mcp.CallToolResult, error) {
	// Convert map to JSON then to InputAlert
	jsonData, err := json.Marshal(data)
	if err != nil {
		return tools.NewToolError("failed to marshal alert data").Cause(err).
			Hint("Check that entity-data contains valid JSON fields").
			Schema("alert", "create").Result()
	}

	var inputAlert thehive.InputCreateAlert
	if err := json.Unmarshal(jsonData, &inputAlert); err != nil {
		return tools.NewToolError("failed to unmarshal alert data").Cause(err).
			Hint("Ensure entity-data fields match the alert schema").
			Schema("alert", "create").Result()
	}

	result, resp, err := client.AlertAPI.CreateAlert(ctx).InputCreateAlert(inputAlert).Execute()
	if err != nil {
		return tools.NewToolError("failed to create alert").Cause(err).
			Hint("Check required fields and permissions").API(resp).Result()
	}

	processedResult, err := parseDateFieldsAndExtractColumns[thehive.OutputAlert](*result, types.DefaultFields[types.EntityTypeAlert])
	if err != nil {
		return tools.NewToolError("failed to parse date fields and extract columns in alert result").Cause(err).Result()
	}

	// For create operations, return the single entity, not an array
	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":  "create",
		"entityType": types.EntityTypeAlert,
		"result":     processedResult,
	}), nil
}

func (t *ManageTool) createCase(ctx context.Context, client *thehive.APIClient, data map[string]interface{}) (*mcp.CallToolResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return tools.NewToolError("failed to marshal case data").Cause(err).
			Hint("Check that entity-data contains valid JSON fields").
			Schema("case", "create").Result()
	}

	var inputCase thehive.InputCreateCase
	if err := json.Unmarshal(jsonData, &inputCase); err != nil {
		return tools.NewToolError("failed to unmarshal case data").Cause(err).
			Hint("Ensure entity-data fields match the case schema").
			Schema("case", "create").Result()
	}

	result, resp, err := client.CaseAPI.CreateCase(ctx).InputCreateCase(inputCase).Execute()
	if err != nil {
		return tools.NewToolError("failed to create case").Cause(err).
			Hint("Check required fields and permissions").API(resp).Result()
	}

	processedResult, err := parseDateFieldsAndExtractColumns[thehive.OutputCase](*result, types.DefaultFields[types.EntityTypeCase])
	if err != nil {
		return tools.NewToolError("failed to parse date fields and extract columns in case result").Cause(err).Result()
	}

	// For create operations, return the single entity, not an array
	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":  "create",
		"entityType": types.EntityTypeCase,
		"result":     processedResult,
	}), nil
}

func (t *ManageTool) createTask(ctx context.Context, client *thehive.APIClient, data map[string]interface{}, parentID string) (*mcp.CallToolResult, error) {

	jsonData, err := json.Marshal(data)
	if err != nil {
		return tools.NewToolError("failed to marshal task data").Cause(err).
			Hint("Check that entity-data contains valid JSON fields").
			Schema("task", "create").Result()
	}

	var inputTask thehive.InputCreateTask
	if err := json.Unmarshal(jsonData, &inputTask); err != nil {
		return tools.NewToolError("failed to unmarshal task data").Cause(err).
			Hint("Ensure entity-data fields match the task schema").
			Schema("task", "create").Result()
	}

	result, resp, err := client.TaskAPI.CreateTaskInCase(ctx, parentID).InputCreateTask(inputTask).Execute()
	if err != nil {
		return tools.NewToolErrorf("failed to create task in case %s", parentID).Cause(err).
			Hint("Check that the case exists and you have permissions").API(resp).Result()
	}

	processedResult, err := parseDateFieldsAndExtractColumns[thehive.OutputTask](*result, types.DefaultFields[types.EntityTypeTask])
	if err != nil {
		return tools.NewToolError("failed to parse date fields and extract columns in task result").Cause(err).Result()
	}

	// For create operations, return the single entity, not an array
	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":  "create",
		"entityType": types.EntityTypeTask,
		"result":     processedResult,
	}), nil
}

func (t *ManageTool) createObservable(ctx context.Context, client *thehive.APIClient, data map[string]interface{}, parentID string) (*mcp.CallToolResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return tools.NewToolError("failed to marshal observable data").Cause(err).
			Hint("Check that entity-data contains valid JSON fields").
			Schema("observable", "create").Result()
	}

	var inputObservable thehive.InputCreateObservable
	if err := json.Unmarshal(jsonData, &inputObservable); err != nil {
		return tools.NewToolError("failed to unmarshal observable data").Cause(err).
			Hint("Ensure entity-data fields match the observable schema").
			Schema("observable", "create").Result()
	}

	// Try to create in case first, then alert if that fails
	var result []thehive.OutputObservable

	// First attempt with case
	caseResult, _, caseErr := client.ObservableAPI.CreateObservableInCase(ctx, parentID).InputCreateObservable(inputObservable).Execute()
	if caseErr != nil {
		// If case creation fails, try alert
		alertResult, _, alertErr := client.ObservableAPI.CreateObservableInAlert(ctx, parentID).InputCreateObservable(inputObservable).Execute()
		if alertErr != nil {
			return tools.NewToolError("failed to create observable").Cause(alertErr).
				Hint("Check that the target case/alert exists and you have permissions").Result()
		}
		result = alertResult
	} else {
		result = caseResult
	}

	processedResult, err := parseDateFieldsAndExtractColumnsFromArray(result, types.DefaultFields[types.EntityTypeObservable])
	if err != nil {
		return tools.NewToolError("failed to parse date fields and extract columns in observable result").Cause(err).Result()
	}

	// For create operations, return the single entity, not an array
	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":  "create",
		"entityType": types.EntityTypeObservable,
		"result":     processedResult, // Return the full array for observables
	}), nil
}
