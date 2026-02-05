package manage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ManageTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 1. Check permissions
	perms, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get permissions: %v", err)), nil
	}

	if !perms.IsToolAllowed("manage-entities") {
		return mcp.NewToolResultError("manage-entities tool is not permitted by your permissions configuration"), nil
	}

	// 2. Extract and validate parameters
	params, err := t.extractParams(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// 3. Check entity operation permissions
	if !perms.IsEntityOperationAllowed(params.EntityType, params.Operation) {
		return mcp.NewToolResultError(fmt.Sprintf("operation '%s' on entity type '%s' is not permitted by your permissions configuration", params.Operation, params.EntityType)), nil
	}

	// 4. Validate operation constraints
	if err := t.validateOperation(params); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// 5. Execute operation
	switch params.Operation {
	case "create":
		return t.handleCreate(ctx, params)
	case "update":
		return t.handleUpdate(ctx, params)
	case "delete":
		return t.handleDelete(ctx, params)
	case "comment":
		return t.handleComment(ctx, params)
	default:
		return mcp.NewToolResultError(fmt.Sprintf("unsupported operation: %s", params.Operation)), nil
	}
}

type manageParams struct {
	Operation  string
	EntityType string
	EntityIDs  []string
	EntityData map[string]interface{}
	Comment    string
}

func filterEntityColumns(entity map[string]interface{}, defaultFields []string) map[string]interface{} {
	filtered := make(map[string]interface{})
	for _, col := range defaultFields {
		if val, exists := entity[col]; exists {
			filtered[col] = val
		}
	}
	return filtered
}
func parseDateFieldsAndExtractColumns[T types.OutputEntity](result T, defaultFields []string) (map[string]interface{}, error) {
	// Handle single entity - wrap it in an array for processing
	processedResult, err := utils.ParseDateFields(result)

	if err != nil {
		return nil, fmt.Errorf("failed to parse date fields: %w", err)
	}
	filtered := filterEntityColumns(processedResult, defaultFields)

	return filtered, nil
}

func parseDateFieldsAndExtractColumnsFromArray[T types.OutputEntity](result []T, defaultFields []string) ([]map[string]interface{}, error) {
	// Handle array of entities
	finalResults := make([]map[string]interface{}, 0, len(result))
	for _, item := range result {
		parsedItem, err := parseDateFieldsAndExtractColumns(item, defaultFields)
		if err != nil {
			return nil, fmt.Errorf("failed to parse date fields in array item: %w", err)
		}
		finalResults = append(finalResults, parsedItem)
	}
	return finalResults, nil
}

func (t *ManageTool) extractParams(req mcp.CallToolRequest) (*manageParams, error) {
	operation := req.GetString("operation", "")
	if operation == "" {
		return nil, fmt.Errorf("operation parameter is required. Must be one of: 'create', 'update', 'delete', 'comment'")
	}

	entityType := req.GetString("entity-type", "")
	if entityType == "" {
		return nil, fmt.Errorf("entity-type parameter is required. Must be one of: 'alert', 'case', 'task', 'observable'")
	}

	params := &manageParams{
		Operation:  operation,
		EntityType: entityType,
		EntityIDs:  req.GetStringSlice("entity-ids", []string{}),
		Comment:    req.GetString("comment", ""),
	}

	// Extract entity data if provided
	if entityDataRaw := req.GetArguments()["entity-data"]; entityDataRaw != nil {
		if entityDataMap, ok := entityDataRaw.(map[string]interface{}); ok {
			params.EntityData = entityDataMap
		} else {
			return nil, fmt.Errorf("entity-data must be a valid JSON object. For schema information, use get-resource 'hive://schema/%s/create' or 'hive://schema/%s/update'", entityType, entityType)
		}
	}

	slog.Info("ManageEntities called",
		"operation", params.Operation,
		"entityType", params.EntityType,
		"entityIDs", params.EntityIDs,
		"hasEntityData", params.EntityData != nil,
		"hasComment", params.Comment != "")

	return params, nil
}

func (t *ManageTool) validateOperation(params *manageParams) error {
	switch params.Operation {
	case "create":
		if params.EntityData == nil {
			return fmt.Errorf("entity-data is required for create operations. Use get-resource 'hive://schema/%s/create' to see required fields for %s creation", params.EntityType, params.EntityType)
		}
		if (params.EntityType == types.EntityTypeTask || params.EntityType == types.EntityTypeObservable) && len(params.EntityIDs) == 0 {
			return fmt.Errorf("%s creation requires a parent case or alert ID in entity-ids parameter", params.EntityType)
		}
		if (params.EntityType == types.EntityTypeTask || params.EntityType == types.EntityTypeObservable) && len(params.EntityIDs) > 1 {
			return fmt.Errorf("%s creation requires exactly one parent ID in entity-ids parameter, got %d", params.EntityType, len(params.EntityIDs))
		}
	case "update":
		if len(params.EntityIDs) == 0 {
			return fmt.Errorf("entity-ids are required for update operations. Provide an array of %s IDs to update, e.g., ['id1', 'id2']", params.EntityType)
		}
		if params.EntityData == nil {
			return fmt.Errorf("entity-data is required for update operations. Provide a JSON object with fields to update. Use get-resource 'hive://schema/%s/update' to see available fields", params.EntityType)
		}
	case "delete":
		if len(params.EntityIDs) == 0 {
			return fmt.Errorf("entity-ids are required for delete operations. Provide an array of %s IDs to delete, e.g., ['id1', 'id2']. WARNING: This operation is irreversible", params.EntityType)
		}
	case "comment":
		if len(params.EntityIDs) == 0 {
			return fmt.Errorf("entity-ids are required for comment operations. Provide an array of %s IDs to add comments to, e.g., ['id1', 'id2']", params.EntityType)
		}
		if params.Comment == "" {
			return fmt.Errorf("comment parameter is required for comment operations. Provide the text content for the comment or task log")
		}
		if params.EntityType != types.EntityTypeCase && params.EntityType != types.EntityTypeTask {
			return fmt.Errorf("comments are only supported on cases and tasks, not %s. For cases: adds a comment. For tasks: adds a task log", params.EntityType)
		}
	}
	return nil
}

// Create operations
func (t *ManageTool) handleCreate(ctx context.Context, params *manageParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get TheHive client: %v. Check your authentication and connection settings.", err)), nil
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
		return mcp.NewToolResultError(fmt.Sprintf("unsupported entity type for create: %s", params.EntityType)), nil
	}
}

func (t *ManageTool) createAlert(ctx context.Context, client *thehive.APIClient, data map[string]interface{}) (*mcp.CallToolResult, error) {
	// Convert map to JSON then to InputAlert
	jsonData, err := json.Marshal(data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal alert data: %v. Check that entity-data contains valid JSON fields. Use get-resource 'hive://schema/alert/create' for field definitions.", err)), nil
	}

	var inputAlert thehive.InputCreateAlert
	if err := json.Unmarshal(jsonData, &inputAlert); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to unmarshal alert data: %v. Ensure entity-data fields match the alert schema. Use get-resource 'hive://schema/alert/create' to see required fields like 'type', 'source', 'sourceRef', 'title', 'description'.", err)), nil
	}

	result, resp, err := client.AlertAPI.CreateAlert(ctx).InputCreateAlert(inputAlert).Execute()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create alert: %v. Check required fields and permissions. API response: %v", err, resp)), nil
	}

	processedResult, err := parseDateFieldsAndExtractColumns[thehive.OutputAlert](*result, types.DefaultFields[types.EntityTypeAlert])
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse date fields and extract columns in alert result: %v", err)), nil
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal case data: %v. Check that entity-data contains valid JSON fields. Use get-resource 'hive://schema/case/create' for field definitions.", err)), nil
	}

	var inputCase thehive.InputCreateCase
	if err := json.Unmarshal(jsonData, &inputCase); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to unmarshal case data: %v. Ensure entity-data fields match the case schema. Use get-resource 'hive://schema/case/create' to see required and optional fields.", err)), nil
	}

	result, resp, err := client.CaseAPI.CreateCase(ctx).InputCreateCase(inputCase).Execute()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create case: %v. Check required fields and permissions. API response: %v", err, resp)), nil
	}

	processedResult, err := parseDateFieldsAndExtractColumns[thehive.OutputCase](*result, types.DefaultFields[types.EntityTypeCase])
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse date fields and extract columns in case result: %v", err)), nil
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal task data: %v. Check that entity-data contains valid JSON fields. Use get-resource 'hive://schema/task/create' for field definitions.", err)), nil
	}

	var inputTask thehive.InputCreateTask
	if err := json.Unmarshal(jsonData, &inputTask); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to unmarshal task data: %v. Ensure entity-data fields match the task schema. Use get-resource 'hive://schema/task/create' to see required and optional fields.", err)), nil
	}

	result, resp, err := client.TaskAPI.CreateTaskInCase(ctx, parentID).InputCreateTask(inputTask).Execute()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create task in case %s: %v. Check that the case exists and you have permissions. API response: %v", parentID, err, resp)), nil
	}

	processedResult, err := parseDateFieldsAndExtractColumns[thehive.OutputTask](*result, types.DefaultFields[types.EntityTypeTask])
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse date fields and extract columns in task result: %v", err)), nil
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal observable data: %v. Check that entity-data contains valid JSON fields. Use get-resource 'hive://schema/observable/create' for field definitions.", err)), nil
	}

	var inputObservable thehive.InputCreateObservable
	if err := json.Unmarshal(jsonData, &inputObservable); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to unmarshal observable data: %v. Ensure entity-data fields match the observable schema. Use get-resource 'hive://schema/observable/create' to see required and optional fields.", err)), nil
	}

	// Try to create in case first, then alert if that fails
	var result []thehive.OutputObservable
	var resp *http.Response
	result, resp, err = client.ObservableAPI.CreateObservableInCase(ctx, parentID).InputCreateObservable(inputObservable).Execute()
	if err != nil {
		// If case creation fails, try alert
		result, resp, err = client.ObservableAPI.CreateObservableInAlert(ctx, parentID).InputCreateObservable(inputObservable).Execute()
	}

	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create observable: %v. Check that the target case/alert exists and you have permissions. API response: %v", err, resp)), nil
	}

	processedResult, err := parseDateFieldsAndExtractColumnsFromArray(result, types.DefaultFields[types.EntityTypeObservable])
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse date fields and extract columns in observable result: %v", err)), nil
	}

	// For create operations, return the single entity, not an array
	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":  "create",
		"entityType": types.EntityTypeObservable,
		"result":     processedResult, // Return the full array for observables
	}), nil
}

// Update operations
func (t *ManageTool) handleUpdate(ctx context.Context, params *manageParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get TheHive client: %v. Check your authentication and connection settings.", err)), nil
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

// Delete operations
func (t *ManageTool) handleDelete(ctx context.Context, params *manageParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get TheHive client: %v. Check your authentication and connection settings.", err)), nil
	}

	results := make([]map[string]interface{}, 0, len(params.EntityIDs))

	for _, entityID := range params.EntityIDs {
		err := t.deleteEntity(ctx, hiveClient, params.EntityType, entityID)
		if err != nil {
			results = append(results, map[string]interface{}{
				"id":    entityID,
				"error": err.Error(),
			})
		} else {
			results = append(results, map[string]interface{}{
				"id":      entityID,
				"deleted": true,
			})
		}
	}

	return utils.NewToolResultJSONUnescaped(map[string]interface{}{
		"operation":  "delete",
		"entityType": params.EntityType,
		"results":    results,
	}), nil
}

func (t *ManageTool) deleteEntity(ctx context.Context, client *thehive.APIClient, entityType, entityID string) error {
	switch entityType {
	case types.EntityTypeAlert:
		resp, err := client.AlertAPI.DeleteAlert(ctx, entityID).Execute()
		if err != nil {
			return fmt.Errorf("failed to delete alert %s: %w. Check that the alert exists and you have permissions. This operation is irreversible. API response: %v", entityID, err, resp)
		}
		return nil

	case types.EntityTypeCase:
		resp, err := client.CaseAPI.DeleteCase(ctx, entityID).Execute()
		if err != nil {
			return fmt.Errorf("failed to delete case %s: %w. Check that the case exists and you have permissions. This operation is irreversible. API response: %v", entityID, err, resp)
		}
		return nil

	case types.EntityTypeTask:
		resp, err := client.TaskAPI.DeleteTask(ctx, entityID).Execute()
		if err != nil {
			return fmt.Errorf("failed to delete task %s: %w. Check that the task exists and you have permissions. This operation is irreversible. API response: %v", entityID, err, resp)
		}
		return nil

	case types.EntityTypeObservable:
		resp, err := client.ObservableAPI.DeleteObservable(ctx, entityID).Execute()
		if err != nil {
			return fmt.Errorf("failed to delete observable %s: %w. Check that the observable exists and you have permissions. This operation is irreversible. API response: %v", entityID, err, resp)
		}
		return nil

	default:
		return fmt.Errorf("unsupported entity type for delete: %s", entityType)
	}
}

// Comment operations
func (t *ManageTool) handleComment(ctx context.Context, params *manageParams) (*mcp.CallToolResult, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get TheHive client: %v. Check your authentication and connection settings.", err)), nil
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
