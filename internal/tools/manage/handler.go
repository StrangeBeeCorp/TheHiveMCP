package manage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ManageTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 1. Check permissions
	perms, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err).Result()
	}

	// 2. Extract and validate parameters
	params, err := t.extractParams(req)
	if err != nil {
		return tools.NewToolError(err.Error()).Result()
	}

	// 3. Check entity operation permissions
	if !perms.IsEntityOperationAllowed(params.EntityType, params.Operation) {
		return tools.NewToolErrorf("operation '%s' on entity type '%s' is not permitted by your permissions configuration", params.Operation, params.EntityType).Result()
	}

	// 4. Validate operation constraints
	if err := t.validateOperation(params); err != nil {
		return tools.NewToolError(err.Error()).Result()
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
	case "promote":
		return t.handlePromote(ctx, params)
	case "merge":
		return t.handleMerge(ctx, params)
	default:
		return tools.NewToolErrorf("unsupported operation: %s", params.Operation).Result()
	}
}

type manageParams struct {
	Operation  string
	EntityType string
	EntityIDs  []string
	EntityData map[string]interface{}
	Comment    string
	TargetID   string
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
		return nil, fmt.Errorf("operation parameter is required. Must be one of: 'create', 'update', 'delete', 'comment', 'promote', 'merge'")
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
		TargetID:   req.GetString("target-id", ""),
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
		"hasComment", params.Comment != "",
		"targetID", params.TargetID)

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
	case "promote":
		if params.EntityType != types.EntityTypeAlert {
			return fmt.Errorf("promote operation is only supported for alerts, not %s. Use promote to convert an alert into a new case", params.EntityType)
		}
		if len(params.EntityIDs) == 0 {
			return fmt.Errorf("entity-ids are required for promote operations. Provide a single alert ID to promote to a case, e.g., ['alert-id']")
		}
		if len(params.EntityIDs) > 1 {
			return fmt.Errorf("promote operation requires exactly one alert ID, got %d. Provide a single alert ID in entity-ids", len(params.EntityIDs))
		}
	case "merge":
		switch params.EntityType {
		case types.EntityTypeCase:
			if len(params.EntityIDs) < 2 {
				return fmt.Errorf("merge operation for cases requires at least 2 case IDs in entity-ids, got %d. Provide multiple case IDs to merge together", len(params.EntityIDs))
			}
		case types.EntityTypeAlert:
			if len(params.EntityIDs) == 0 {
				return fmt.Errorf("merge operation for alerts requires alert IDs in entity-ids. Provide alert IDs to merge into the target case")
			}
			if params.TargetID == "" {
				return fmt.Errorf("merge operation for alerts requires target-id parameter. Provide the case ID to merge alerts into")
			}
		case types.EntityTypeObservable:
			if params.TargetID == "" {
				return fmt.Errorf("merge operation for observables requires target-id parameter. Provide the case ID containing observables to deduplicate")
			}
		default:
			return fmt.Errorf("merge operation is not supported for entity type %s. Merge is only supported for cases, alerts, and observables", params.EntityType)
		}
	}
	return nil
}
