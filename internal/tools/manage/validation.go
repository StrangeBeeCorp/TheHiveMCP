package manage

import (
	"context"
	"log/slog"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// ValidatePermissions checks that the caller is allowed to run the requested operation on the target entities.
func (t *Tool) ValidatePermissions(ctx context.Context, params EntityParams) error {
	perms, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return tools.NewToolError("failed to get permissions").Cause(err)
	}

	if !perms.IsToolAllowed(t.Name()) {
		return tools.NewToolErrorf("tool %s is not permitted by your permissions configuration", t.Name())
	}

	if !perms.IsEntityOperationAllowed(params.EntityType, params.Operation) {
		return tools.NewToolErrorf("operation '%s' on entity type '%s' is not permitted by your permissions configuration", params.Operation, params.EntityType)
	}

	return t.validateEntityScope(ctx, perms, params)
}

// scopeCheck names entities an operation reaches, to verify against the
// manage-entities permission filters before any mutation.
type scopeCheck struct {
	entityType string
	entityIDs  []string
}

// validateEntityScope denies by-ID operations on entities that the configured
// manage-entities permission filters exclude (DL-6004). With no configured
// filters every operation proceeds unchanged.
func (t *Tool) validateEntityScope(ctx context.Context, perms *permissions.Config, params EntityParams) error {
	permFilters := perms.GetToolFilters(t.Name())
	if len(permFilters) == 0 {
		return nil
	}

	allOf, anyOf := scopeChecksForOperation(params)

	err := checkAllInScope(ctx, allOf, permFilters)
	if err != nil {
		return err
	}

	return checkAnyInScope(ctx, anyOf, permFilters)
}

func checkAllInScope(ctx context.Context, checks []scopeCheck, permFilters map[string]any) error {
	for _, check := range checks {
		// Tolerant get-by-idOrName resolution: manage entity-ids are user/LLM-supplied
		// and may be bare (a plain case number, not the ~-prefixed _id), which the
		// batched _in{_id} query would wrongly deny (DL-5764).
		inScope, err := utils.ScopedEntityIDsTolerant(ctx, check.entityType, check.entityIDs, permFilters)
		if err != nil {
			return tools.NewToolError("failed to verify entity scope").Cause(err).
				Hint("The operation was denied because the configured permission filters could not be checked against the target entities")
		}

		for _, entityID := range check.entityIDs {
			if !inScope[entityID] {
				return scopeDeniedError(check.entityType, entityID)
			}
		}
	}

	return nil
}

// checkAnyInScope passes if at least one alternative is in scope (parent may be
// a case OR an alert). Query error counts as non-match; none matching denies
// (fail closed). No alternatives passes.
func checkAnyInScope(ctx context.Context, checks []scopeCheck, permFilters map[string]any) error {
	if len(checks) == 0 {
		return nil
	}

	for _, check := range checks {
		inScope, err := utils.IsEntityInScopeTolerant(ctx, check.entityType, check.entityIDs[0], permFilters)
		if err != nil {
			slog.Debug("Entity scope alternative check failed", "entityType", check.entityType, "error", err)
			continue
		}

		if inScope {
			return nil
		}
	}

	return scopeDeniedError("parent entity", checks[0].entityIDs[0])
}

func scopeDeniedError(entityType, entityID string) error {
	return tools.NewToolErrorf("%s %s was not found or is not within the scope permitted by your permissions configuration", entityType, entityID).
		Hint("The configured permission filters restrict which entities this tool can reach")
}

// scopeChecksForOperation maps an operation to the existing entities it reaches.
// Every allOf entry must be in scope; anyOf entries (case-or-alert parents) need
// one match. Top-level creation reaches no existing entity: no checks.
func scopeChecksForOperation(params EntityParams) (allOf, anyOf []scopeCheck) {
	switch params.Operation {
	case OperationCreate:
		if len(params.EntityIDs) == 0 {
			return nil, nil
		}

		parentID := params.EntityIDs[0]
		switch params.EntityType {
		case types.EntityTypeTask, types.EntityTypePage:
			allOf = append(allOf, scopeCheck{types.EntityTypeCase, []string{parentID}})
		case types.EntityTypeObservable, types.EntityTypeProcedure:
			anyOf = append(anyOf,
				scopeCheck{types.EntityTypeCase, []string{parentID}},
				scopeCheck{types.EntityTypeAlert, []string{parentID}},
			)
		}
	case OperationUpdate, OperationDelete, OperationComment:
		allOf = append(allOf, scopeCheck{params.EntityType, params.EntityIDs})
		// Case-attached pages are addressed together with their parent case
		if params.EntityType == types.EntityTypePage && params.TargetID != "" {
			allOf = append(allOf, scopeCheck{types.EntityTypeCase, []string{params.TargetID}})
		}
	case OperationPromote:
		allOf = append(allOf, scopeCheck{types.EntityTypeAlert, params.EntityIDs})
	case OperationMerge:
		switch params.EntityType {
		case types.EntityTypeCase:
			allOf = append(allOf, scopeCheck{types.EntityTypeCase, params.EntityIDs})
		case types.EntityTypeAlert:
			allOf = append(allOf,
				scopeCheck{types.EntityTypeAlert, params.EntityIDs},
				scopeCheck{types.EntityTypeCase, []string{params.TargetID}},
			)
		case types.EntityTypeObservable:
			allOf = append(allOf, scopeCheck{types.EntityTypeCase, []string{params.TargetID}})
		}
	case OperationApplyTemplate:
		// Only the cases being modified are scoped; the template target is
		// org-level configuration, not filterable row data (see
		// docs/reference/permissions.md for this documented limitation).
		allOf = append(allOf, scopeCheck{types.EntityTypeCase, params.EntityIDs})
	}

	return allOf, anyOf
}

// ValidateParams validates the tool parameters for the requested operation and entity type.
func (t *Tool) ValidateParams(params *EntityParams) error {
	switch params.Operation {
	case OperationCreate:
		return validateCreateParams(params)
	case OperationUpdate:
		return validateUpdateParams(params)
	case OperationDelete:
		return validateDeleteParams(params)
	case OperationComment:
		return validateCommentParams(params)
	case OperationPromote:
		return validatePromoteParams(params)
	case OperationMerge:
		return validateMergeParams(params)
	case OperationApplyTemplate:
		return validateApplyTemplateParams(params)
	}

	return nil
}

func validateCreateParams(params *EntityParams) error {
	if params.EntityData == nil {
		return tools.NewToolError("entity-data is required for create operations.").Hintf(
			"Use get-resource 'hive://schema/%s/create' to see required fields for %s creation", params.EntityType, params.EntityType)
	}

	needsParentID := params.EntityType == types.EntityTypeTask || params.EntityType == types.EntityTypeObservable || params.EntityType == types.EntityTypeProcedure
	if needsParentID && len(params.EntityIDs) == 0 {
		return tools.NewToolErrorf("%s creation requires a parent case or alert ID in entity-ids parameter", params.EntityType)
	}

	if needsParentID && len(params.EntityIDs) > 1 {
		return tools.NewToolErrorf("%s creation requires exactly one parent ID in entity-ids parameter, got %d", params.EntityType, len(params.EntityIDs))
	}
	// Pages support an optional parent case ID
	if params.EntityType == types.EntityTypePage && len(params.EntityIDs) > 1 {
		return tools.NewToolErrorf("page creation accepts at most one parent case ID in entity-ids parameter, got %d", len(params.EntityIDs))
	}

	return nil
}

func validateUpdateParams(params *EntityParams) error {
	if len(params.EntityIDs) == 0 {
		return tools.NewToolErrorf("entity-ids are required for update operations. Provide an array of %s IDs to update, e.g., ['id1', 'id2']", params.EntityType)
	}

	if params.EntityData == nil {
		return tools.NewToolErrorf("entity-data is required for update operations. Provide a JSON object with fields to update.").Hintf(
			"Use get-resource 'hive://schema/%s/update' to see available fields", params.EntityType)
	}

	return nil
}

func validateDeleteParams(params *EntityParams) error {
	if len(params.EntityIDs) == 0 {
		return tools.NewToolErrorf("entity-ids are required for delete operations. Provide an array of %s IDs to delete, e.g., ['id1', 'id2']. WARNING: This operation is irreversible", params.EntityType)
	}

	return nil
}

func validateCommentParams(params *EntityParams) error {
	if len(params.EntityIDs) == 0 {
		return tools.NewToolErrorf("entity-ids are required for comment operations. Provide an array of %s IDs to add comments to, e.g., ['id1', 'id2']", params.EntityType)
	}

	if params.Comment == "" {
		return tools.NewToolError("comment parameter is required for comment operations. Provide the text content for the comment or task log")
	}

	if params.EntityType != types.EntityTypeCase && params.EntityType != types.EntityTypeTask {
		return tools.NewToolErrorf("comments are only supported on cases and tasks, not %s. For cases: adds a comment. For tasks: adds a task log", params.EntityType)
	}

	return nil
}

func validatePromoteParams(params *EntityParams) error {
	if params.EntityType != types.EntityTypeAlert {
		return tools.NewToolErrorf("promote operation is only supported for alerts, not %s. Use promote to convert an alert into a new case", params.EntityType)
	}

	if len(params.EntityIDs) == 0 {
		return tools.NewToolErrorf("entity-ids are required for promote operations. Provide a single alert ID to promote to a case, e.g., ['alert-id']")
	}

	if len(params.EntityIDs) > 1 {
		return tools.NewToolErrorf("promote operation requires exactly one alert ID, got %d. Provide a single alert ID in entity-ids", len(params.EntityIDs))
	}

	return nil
}

func validateMergeParams(params *EntityParams) error {
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

	return nil
}

func validateApplyTemplateParams(params *EntityParams) error {
	if params.EntityType != types.EntityTypeCase {
		return tools.NewToolErrorf("apply-template operation is only supported for cases, not %s. Use entity-type=\"case\" and provide case IDs in entity-ids", params.EntityType)
	}

	if len(params.EntityIDs) == 0 {
		return tools.NewToolError("entity-ids are required for apply-template operations. Provide an array of case IDs to apply the template to")
	}

	if params.TargetID == "" {
		return tools.NewToolError("target-id is required for apply-template operations. Provide the case template name or ID to apply")
	}

	return nil
}
