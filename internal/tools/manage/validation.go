package manage

import (
	"context"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

func (t *ManageTool) ValidatePermissions(ctx context.Context, params ManageEntityParams) error {
	permissions, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err)
	}

	if !permissions.IsToolAllowed(t.Name()) {
		return tools.NewToolErrorf("tool %s is not allowed by your permissions configuration", t.Name())
	}

	if !permissions.IsEntityOperationAllowed(params.EntityType, params.Operation) {
		return tools.NewToolErrorf("operation '%s' on entity type '%s' is not permitted by your permissions configuration", params.Operation, params.EntityType)
	}

	return nil
}

func (t *ManageTool) ValidateParams(params *ManageEntityParams) error {
	switch params.Operation {
	case OperationCreate:
		if params.EntityData == nil {
			return tools.NewToolError("entity-data is required for create operations.").Hintf(
				"Use get-resource 'hive://schema/%s/create' to see required fields for %s creation", params.EntityType, params.EntityType)
		}
		if (params.EntityType == types.EntityTypeTask || params.EntityType == types.EntityTypeObservable) && len(params.EntityIDs) == 0 {
			return tools.NewToolErrorf("%s creation requires a parent case or alert ID in entity-ids parameter", params.EntityType)
		}
		if (params.EntityType == types.EntityTypeTask || params.EntityType == types.EntityTypeObservable) && len(params.EntityIDs) > 1 {
			return tools.NewToolErrorf("%s creation requires exactly one parent ID in entity-ids parameter, got %d", params.EntityType, len(params.EntityIDs))
		}
	case OperationUpdate:
		if len(params.EntityIDs) == 0 {
			return tools.NewToolErrorf("entity-ids are required for update operations. Provide an array of %s IDs to update, e.g., ['id1', 'id2']", params.EntityType)
		}
		if params.EntityData == nil {
			return tools.NewToolErrorf("entity-data is required for update operations. Provide a JSON object with fields to update.").Hintf(
				"Use get-resource 'hive://schema/%s/update' to see available fields", params.EntityType)
		}
	case OperationDelete:
		if len(params.EntityIDs) == 0 {
			return tools.NewToolErrorf("entity-ids are required for delete operations. Provide an array of %s IDs to delete, e.g., ['id1', 'id2']. WARNING: This operation is irreversible", params.EntityType)
		}
	case OperationComment:
		if len(params.EntityIDs) == 0 {
			return tools.NewToolErrorf("entity-ids are required for comment operations. Provide an array of %s IDs to add comments to, e.g., ['id1', 'id2']", params.EntityType)
		}
		if params.Comment == "" {
			return tools.NewToolError("comment parameter is required for comment operations. Provide the text content for the comment or task log")
		}
		if params.EntityType != types.EntityTypeCase && params.EntityType != types.EntityTypeTask {
			return tools.NewToolErrorf("comments are only supported on cases and tasks, not %s. For cases: adds a comment. For tasks: adds a task log", params.EntityType)
		}
	case OperationPromote:
		if params.EntityType != types.EntityTypeAlert {
			return tools.NewToolErrorf("promote operation is only supported for alerts, not %s. Use promote to convert an alert into a new case", params.EntityType)
		}
		if len(params.EntityIDs) == 0 {
			return tools.NewToolErrorf("entity-ids are required for promote operations. Provide a single alert ID to promote to a case, e.g., ['alert-id']")
		}
		if len(params.EntityIDs) > 1 {
			return tools.NewToolErrorf("promote operation requires exactly one alert ID, got %d. Provide a single alert ID in entity-ids", len(params.EntityIDs))
		}
	case OperationMerge:
		switch params.EntityType {
		case types.EntityTypeCase:
			if len(params.EntityIDs) < 2 {
				return tools.NewToolErrorf("merge operation for cases requires at least 2 case IDs in entity-ids, got %d. Provide multiple case IDs to merge together", len(params.EntityIDs))
			}
		case types.EntityTypeAlert:
			if len(params.EntityIDs) == 0 {
				return tools.NewToolErrorf("merge operation for alerts requires alert IDs in entity-ids. Provide alert IDs to merge into the target case")
			}
			if params.TargetID == "" {
				return tools.NewToolErrorf("merge operation for alerts requires target-id parameter. Provide the case ID to merge alerts into")
			}
		case types.EntityTypeObservable:
			if params.TargetID == "" {
				return tools.NewToolErrorf("merge operation for observables requires target-id parameter. Provide the case ID containing observables to deduplicate")
			}
		default:
			return tools.NewToolErrorf("merge operation is not supported for entity type %s. Merge is only supported for cases, alerts, and observables", params.EntityType)
		}
	}
	return nil
}
