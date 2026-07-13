package utils

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"unicode"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// ListOperationName returns the TheHive list operation for a type ("case" -> "listCase").
func ListOperationName(entityType string) string {
	return entityOperationName("list", entityType)
}

// getOperationName: "case" -> "getCase" (fetches one entity by ID or name).
func getOperationName(entityType string) string {
	return entityOperationName("get", entityType)
}

func entityOperationName(verb, entityType string) string {
	if entityType == "" {
		return ""
	}

	capitalizedOverrides := map[string]string{
		types.EntityTypeCaseTemplate: "CaseTemplate",
	}

	capitalizedEntityType, ok := capitalizedOverrides[entityType]
	if !ok {
		capitalizedEntityType = string(unicode.ToUpper(rune(entityType[0]))) + entityType[1:]
	}

	return fmt.Sprintf("%s%s", verb, capitalizedEntityType)
}

// GetEntityIDsInScope reports which entity IDs match permFilters for the given
// type, resolved server-side in one list query. With no filters every ID is in
// scope. See scopedEntityIDsBatch for the query shape.
func GetEntityIDsInScope(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]any) (map[string]bool, error) {
	return scopedEntityIDsBatch(ctx, entityType, entityIDs, permFilters)
}

// GetScopedEntityIDsBatch is an alias of GetEntityIDsInScope: both resolve the
// in-scope set with the same single list query (DL-5764). The distinct name is
// kept because callers document intent as a batch re-check on the
// similarity-expansion path.
func GetScopedEntityIDsBatch(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]any) (map[string]bool, error) {
	return scopedEntityIDsBatch(ctx, entityType, entityIDs, permFilters)
}

// scopedEntityIDsBatch reports which of entityIDs match permFilters for the
// given type, in a single list query:
//
//	listX -> filter(_and[ permFilters, _in{_field:_id, _values:ids} ]) -> page(0, len(ids))
//
// _in on _id is honoured by TheHive on a list op for every type that reaches
// here (case/alert/task/observable/page/procedure), returning exactly the same
// set as N get-by-ID checks — verified live against 5.6.3 and pinned by
// TestGetScopedEntityIDsBatchHonorsIDFilter, which diffs this path against the
// get-by-ID oracle (ScopedEntityIDsByGetOneByOne). This replaces the earlier
// per-ID fan-out: one round-trip instead of N, same result.
//
// Fail-closed: any query error returns an error and no map, never a partial map
// treated as complete. The result seeds every requested id to false, then flips
// the ones the query returned to true; an id never requested is never added.
func scopedEntityIDsBatch(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]any) (map[string]bool, error) {
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

	// Seed every requested id to false so an out-of-scope id is present-and-false,
	// not absent (matches the get-by-ID oracle's result shape). The query below
	// flips the matched ones to true.
	for _, id := range entityIDs {
		inScope[id] = false
	}

	listOpName := ListOperationName(entityType)
	if listOpName == "" {
		return nil, errors.New("cannot verify scope: missing entity type")
	}

	listOp := map[string]any{opNameKey: listOpName}
	filterOp := scopeInFilterOperation(permFilters, entityIDs)
	// Explicit page so a batch larger than TheHive's default window is not
	// truncated into a false "out of scope" (a denial bug, not a leak).
	pageOp := thehive.NewInputQueryPagingOperation(0, int32(len(entityIDs)), "page") // #nosec G115 -- entityIDs length is bounded by the caller's hit set

	matchedIDs, err := executeScopeQueryIDs(ctx, []thehive.InputQueryNamedOperation{
		thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&listOp),
		thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&filterOp),
		thehive.InputQueryPagingOperationAsInputQueryNamedOperation(pageOp),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to verify %s IDs against permission filters: %w", entityType, err)
	}

	for _, id := range entityIDs {
		if _, ok := matchedIDs[id]; ok {
			inScope[id] = true
		}
	}

	return inScope, nil
}

// ScopedEntityIDsByGetOneByOne is the earlier per-ID get-by-ID implementation,
// kept only as the differential oracle for TestGetScopedEntityIDsBatchHonorsIDFilter:
// each id resolves through the canonical getX -> filter(permFilters) fetch-by-_id
// path. Production code no longer calls this — GetEntityIDsInScope /
// GetScopedEntityIDsBatch use the single-query _in path. It is exported solely so
// the live integration test in package manage_test can diff the _in path against
// this independent, proven implementation (do not use in production code).
func ScopedEntityIDsByGetOneByOne(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]any) (map[string]bool, error) {
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

	getOpName := getOperationName(entityType)
	if getOpName == "" {
		return nil, errors.New("cannot verify scope: missing entity type")
	}

	filterOp := scopeFilterOperation(permFilters)

	for _, entityID := range entityIDs {
		getOp := map[string]any{
			opNameKey:   getOpName,
			idOrNameKey: entityID,
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

// IsJobObservableInScope reports whether a Cortex job's target observable matches
// permFilters, resolved server-side (getJob -> observable -> filter). No filters:
// every job is in scope.
func IsJobObservableInScope(ctx context.Context, jobID string, permFilters map[string]any) (bool, error) {
	if len(permFilters) == 0 {
		return true, nil
	}

	getJobOp := map[string]any{
		opNameKey:   "getJob",
		idOrNameKey: jobID,
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

func scopeFilterOperation(permFilters map[string]any) map[string]any {
	filterOp := make(map[string]any, len(permFilters)+1)
	maps.Copy(filterOp, permFilters)

	filterOp = NormalizeFilterKeys(filterOp)
	filterOp = TranslateDatesToTimestamps(filterOp)
	filterOp[opNameKey] = opFilter

	return filterOp
}

// scopeInFilterOperation builds filter(_and[ permFilters, _in{_id: ids} ]).
// permFilters is nested whole (like permissions.MergeFilters) rather than
// key-merged, since it may itself be an _and/_or. Only permFilters is normalized
// and date-translated; the _in clause is left untouched so the ~-prefixed ids
// reach TheHive byte-for-byte.
func scopeInFilterOperation(permFilters map[string]any, entityIDs []string) map[string]any {
	normalized := make(map[string]any, len(permFilters))
	maps.Copy(normalized, permFilters)

	normalized = NormalizeFilterKeys(normalized)
	normalized = TranslateDatesToTimestamps(normalized)

	idValues := make([]any, len(entityIDs))
	for i, id := range entityIDs {
		idValues[i] = id
	}

	return map[string]any{
		opNameKey: opFilter,
		opAnd: []any{
			normalized,
			map[string]any{opIn: map[string]any{keyField: fieldID, keyValues: idValues}},
		},
	}
}

func executeScopeQuery(ctx context.Context, operations []thehive.InputQueryNamedOperation) (bool, error) {
	hiveClient, err := GetHiveClientFromContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get TheHive client: %w", err)
	}

	hiveQuery := thehive.InputQuery{Query: operations}

	results, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if err != nil {
		return false, fmt.Errorf("scope query failed: %w. API response: %v", err, resp)
	}

	resultsSlice, ok := results.([]any)
	if !ok {
		return false, fmt.Errorf("unexpected result type from scope query. Expected []interface{} but got %T", results)
	}

	return len(resultsSlice) > 0, nil
}

// executeScopeQueryIDs runs a scope list query and returns the set of top-level
// _id values of the returned rows. Fail-closed: any error returns a nil set and
// the error, never a partial set.
func executeScopeQueryIDs(ctx context.Context, operations []thehive.InputQueryNamedOperation) (map[string]struct{}, error) {
	hiveClient, err := GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client: %w", err)
	}

	hiveQuery := thehive.InputQuery{Query: operations}

	results, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if err != nil {
		return nil, fmt.Errorf("scope query failed: %w. API response: %v", err, resp)
	}

	resultsSlice, ok := results.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from scope query. Expected []interface{} but got %T", results)
	}

	ids := make(map[string]struct{}, len(resultsSlice))
	for _, row := range resultsSlice {
		rowMap, ok := row.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unexpected scope query row type. Expected map[string]interface{} but got %T", row)
		}

		id, ok := rowMap[fieldID].(string)
		if !ok {
			return nil, fmt.Errorf("scope query row missing string %q field", fieldID)
		}

		ids[id] = struct{}{}
	}

	return ids, nil
}

// IsEntityInScope is the single-ID form of GetEntityIDsInScope.
func IsEntityInScope(ctx context.Context, entityType, entityID string, permFilters map[string]any) (bool, error) {
	inScope, err := GetEntityIDsInScope(ctx, entityType, []string{entityID}, permFilters)
	if err != nil {
		return false, err
	}

	return inScope[entityID], nil
}
