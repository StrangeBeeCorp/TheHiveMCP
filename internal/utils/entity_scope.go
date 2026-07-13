package utils

import (
	"context"
	"errors"
	"fmt"
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
//
// PRECONDITION: every id must be the ~-prefixed internal _id form. The single
// _in{_id} query does NOT match a bare id (a plain case number or a name), even
// one that get-by-idOrName would resolve — TheHive only honours _in on _id for
// the ~-prefixed form (DL-5764). This is safe for the similarity-expansion path,
// whose ids come straight from a prior search's _id field. Callers that accept
// user/LLM-supplied ids (which may be bare) must use ScopedEntityIDsTolerant
// instead, which resolves each id through the more forgiving get-by-idOrName path.
func GetEntityIDsInScope(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]any) (map[string]bool, error) {
	return scopedEntityIDsBatch(ctx, entityType, entityIDs, permFilters)
}

// ScopedEntityIDsTolerant reports which entity IDs match permFilters for the
// given type, resolving each id through the get-by-idOrName path (getX
// idOrName=<id> -> filter(permFilters)). Unlike GetEntityIDsInScope's single
// _in{_id} query, this tolerates BARE ids — a plain case number or a name that
// get-by-idOrName accepts, not only the ~-prefixed internal _id (DL-5764
// regression).
//
// Use this on the CRUD/manage and execute-automation paths, where entity ids are
// supplied by the user/LLM and may be bare. It costs N round-trips (one per id)
// instead of one; those paths address only a handful of ids, so the tolerance is
// worth the extra queries. The batched _in path stays on the similarity-expansion
// fan-out, where ids are provably ~-prefixed and the N-hit round-trip saving matters.
func ScopedEntityIDsTolerant(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]any) (map[string]bool, error) {
	return ScopedEntityIDsByGetOneByOne(ctx, entityType, entityIDs, permFilters)
}

// IsEntityInScopeTolerant is the single-ID form of ScopedEntityIDsTolerant: it
// resolves the id through the bare-id-tolerant get-by-idOrName path.
func IsEntityInScopeTolerant(ctx context.Context, entityType, entityID string, permFilters map[string]any) (bool, error) {
	inScope, err := ScopedEntityIDsTolerant(ctx, entityType, []string{entityID}, permFilters)
	if err != nil {
		return false, err
	}

	return inScope[entityID], nil
}

// GetScopedEntityIDsBatch is an alias of GetEntityIDsInScope: both resolve the
// in-scope set with the same single list query (DL-5764). The distinct name is
// kept because callers document intent as a batch re-check on the
// similarity-expansion path.
func GetScopedEntityIDsBatch(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]any) (map[string]bool, error) {
	return scopedEntityIDsBatch(ctx, entityType, entityIDs, permFilters)
}

// scopedEntityIDsBatch reports which of entityIDs match permFilters for the
// given type, via one list query per chunk of at most scopeBatchChunkSize ids:
//
//	listX -> filter(_and[ permFilters, _in{_field:_id, _values:chunk} ]) -> page(0, len(chunk))
//
// _in on _id is honoured by TheHive on a list op for every type that reaches
// here (case/alert/task/observable/page/procedure), returning exactly the same
// set as N get-by-ID checks — verified live against 5.6.3 and pinned by
// TestGetScopedEntityIDsBatchHonorsIDFilter, which diffs this path against the
// get-by-ID oracle (ScopedEntityIDsByGetOneByOne). This replaces the earlier
// per-ID fan-out: ceil(N/chunk) round-trips instead of N, same result.
//
// Chunking (scopeBatchChunkSize) keeps each _in clause below any server-side
// clause/row cap: an oversized single query could 400 (fail closed on the whole
// batch) or silently truncate its result rows, which seeds the dropped ids false
// and mass-denies legitimately in-scope hits (DL-5764). The matched sets of all
// chunks are unioned, so the split is invisible to the caller.
//
// Fail-closed: any chunk query error returns an error and no map, never a partial
// map treated as complete. The result seeds every requested id to false, then
// flips the ones the queries returned to true; an id never requested is never added.
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
	// not absent (matches the get-by-ID oracle's result shape). The chunk queries
	// below flip the matched ones to true.
	for _, id := range entityIDs {
		inScope[id] = false
	}

	listOpName := ListOperationName(entityType)
	if listOpName == "" {
		return nil, errors.New("cannot verify scope: missing entity type")
	}

	for start := 0; start < len(entityIDs); start += scopeBatchChunkSize {
		end := min(start+scopeBatchChunkSize, len(entityIDs))
		chunk := entityIDs[start:end]

		matchedIDs, err := scopedEntityIDsChunk(ctx, listOpName, chunk, permFilters)
		if err != nil {
			// Batching collapses one query per id into one query per chunk, so the
			// error can't name the single failing id the per-ID oracle would. Carry
			// the entity type and id counts (this chunk, and the whole batch) instead,
			// so a failed denial-path check is still triageable. The underlying
			// executeScopeQueryIDs wrap preserves the API response detail. The ids
			// themselves are not logged — they can be large in number and are not
			// needed to identify which check failed.
			return nil, fmt.Errorf("failed to verify %d %s IDs (chunk ids %d-%d of %d total) against permission filters: %w",
				len(chunk), entityType, start, end-1, len(entityIDs), err)
		}

		for _, id := range chunk {
			if _, ok := matchedIDs[id]; ok {
				inScope[id] = true
			}
		}
	}

	return inScope, nil
}

// scopedEntityIDsChunk runs the single list query for one bounded chunk of ids
// and returns the matched _id set. chunk is expected to hold at most
// scopeBatchChunkSize ids; callers do the chunking.
func scopedEntityIDsChunk(ctx context.Context, listOpName string, chunk []string, permFilters map[string]any) (map[string]struct{}, error) {
	listOp := map[string]any{opNameKey: listOpName}
	filterOp := scopeInFilterOperation(permFilters, chunk)
	// Explicit page so a chunk larger than TheHive's default window is not
	// truncated into a false "out of scope" (a denial bug, not a leak).
	pageOp := thehive.NewInputQueryPagingOperation(0, int32(len(chunk)), "page") // #nosec G115 -- chunk length is bounded by scopeBatchChunkSize

	return executeScopeQueryIDs(ctx, []thehive.InputQueryNamedOperation{
		thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&listOp),
		thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&filterOp),
		thehive.InputQueryPagingOperationAsInputQueryNamedOperation(pageOp),
	})
}

// ScopedEntityIDsByGetOneByOne resolves scope per id through the canonical getX
// idOrName=<id> -> filter(permFilters) path. get-by-idOrName tolerates bare ids
// (a plain case number or a name), where the batched _in{_id} query matches only
// the ~-prefixed internal _id (DL-5764).
//
// It is the implementation behind ScopedEntityIDsTolerant (the CRUD/manage and
// execute-automation paths) and also the independent differential oracle for
// TestGetScopedEntityIDsBatchHonorsIDFilter, which diffs the _in path against this
// one. Prefer the ScopedEntityIDsTolerant / IsEntityInScopeTolerant wrappers in
// callers so intent reads at the call site; this stays exported for the test.
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
	filterOp := deepCopyFilter(permFilters)

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
	normalized := deepCopyFilter(permFilters)

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

// IsEntityInScope is the single-ID form of GetEntityIDsInScope and shares its
// ~-prefixed-id precondition. For a possibly-bare user/LLM id use
// IsEntityInScopeTolerant.
func IsEntityInScope(ctx context.Context, entityType, entityID string, permFilters map[string]any) (bool, error) {
	inScope, err := GetEntityIDsInScope(ctx, entityType, []string{entityID}, permFilters)
	if err != nil {
		return false, err
	}

	return inScope[entityID], nil
}
