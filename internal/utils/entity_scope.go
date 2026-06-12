package utils

import (
	"context"
	"fmt"
	"unicode"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// ListOperationName returns the TheHive query operation that lists entities
// of the given type (e.g. "case" -> "listCase").
func ListOperationName(entityType string) string {
	return entityOperationName("list", entityType)
}

// getOperationName returns the TheHive query operation that fetches a single
// entity of the given type by ID or name (e.g. "case" -> "getCase").
func getOperationName(entityType string) string {
	return entityOperationName("get", entityType)
}

func entityOperationName(verb, entityType string) string {
	if entityType == "" {
		return ""
	}
	// Handle entity types that don't follow simple capitalization
	capitalizedOverrides := map[string]string{
		types.EntityTypeCaseTemplate: "CaseTemplate",
	}
	capitalizedEntityType, ok := capitalizedOverrides[entityType]
	if !ok {
		capitalizedEntityType = string(unicode.ToUpper(rune(entityType[0]))) + entityType[1:]
	}
	return fmt.Sprintf("%s%s", verb, capitalizedEntityType)
}

// GetEntityIDsInScope reports which of the given entity IDs match the
// permission filters for the given entity type. Each ID is checked
// server-side by fetching the entity with the permission filters applied as
// a filter stage, so an entity excluded by the filters is reported exactly
// like a missing one. (TheHive does not support _id equality filters on
// list operations for every entity type, so a get-by-ID pipeline is used
// instead of one batched list query.)
// With no configured filters every requested ID is in scope.
func GetEntityIDsInScope(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]interface{}) (map[string]bool, error) {
	inScope := make(map[string]bool, len(entityIDs))
	if len(permFilters) == 0 {
		for _, id := range entityIDs {
			inScope[id] = true
		}
		return inScope, nil
	}
	if len(entityIDs) == 0 {
		return inScope, nil
	}

	filterOp := scopeFilterOperation(permFilters)
	for _, entityID := range entityIDs {
		getOp := map[string]interface{}{
			"_name":    getOperationName(entityType),
			"idOrName": entityID,
		}

		matched, err := executeScopeQuery(ctx, []thehive.InputQueryNamedOperation{
			thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&getOp),
			thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&filterOp),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to verify %s %s against permission filters: %w", entityType, entityID, err)
		}
		inScope[entityID] = matched
	}

	return inScope, nil
}

// IsJobObservableInScope reports whether the observable targeted by the given
// Cortex analyzer job matches the permission filters. The job's target is
// resolved server-side (getJob -> observable traversal) and the filters are
// applied to the observable in the same query, so a job whose observable the
// filters exclude is reported exactly like a missing one.
// With no configured filters every job is in scope.
func IsJobObservableInScope(ctx context.Context, jobID string, permFilters map[string]interface{}) (bool, error) {
	if len(permFilters) == 0 {
		return true, nil
	}

	getJobOp := map[string]interface{}{
		"_name":    "getJob",
		"idOrName": jobID,
	}
	filterOp := scopeFilterOperation(permFilters)

	matched, err := executeScopeQuery(ctx, []thehive.InputQueryNamedOperation{
		thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&getJobOp),
		thehive.InputQueryGenericOperationAsInputQueryNamedOperation(thehive.NewInputQueryGenericOperation("observable")),
		thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&filterOp),
	})
	if err != nil {
		return false, fmt.Errorf("failed to verify job %s against permission filters: %w", jobID, err)
	}
	return matched, nil
}

// scopeFilterOperation turns permission filters into a TheHive filter stage.
func scopeFilterOperation(permFilters map[string]interface{}) map[string]interface{} {
	filterOp := make(map[string]interface{}, len(permFilters)+1)
	for k, v := range permFilters {
		filterOp[k] = v
	}
	filterOp = TranslateDatesToTimestamps(filterOp)
	filterOp["_name"] = "filter"
	return filterOp
}

// executeScopeQuery runs a scope-check query and reports whether it matched
// anything.
func executeScopeQuery(ctx context.Context, operations []thehive.InputQueryNamedOperation) (bool, error) {
	hiveClient, err := GetHiveClientFromContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get TheHive client: %w", err)
	}

	hiveQuery := thehive.InputQuery{Query: operations}
	results, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()
	if err != nil {
		return false, fmt.Errorf("scope query failed: %w. API response: %v", err, resp)
	}

	resultsSlice, ok := results.([]interface{})
	if !ok {
		return false, fmt.Errorf("unexpected result type from scope query. Expected []interface{} but got %T", results)
	}
	return len(resultsSlice) > 0, nil
}

// IsEntityInScope reports whether a single entity matches the permission
// filters for the given entity type. See GetEntityIDsInScope.
func IsEntityInScope(ctx context.Context, entityType, entityID string, permFilters map[string]interface{}) (bool, error) {
	inScope, err := GetEntityIDsInScope(ctx, entityType, []string{entityID}, permFilters)
	if err != nil {
		return false, err
	}
	return inScope[entityID], nil
}
