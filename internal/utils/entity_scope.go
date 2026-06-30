package utils

import (
	"context"
	"fmt"
	"sync"
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
	return scopedEntityIDsBatch(ctx, entityType, entityIDs, permFilters, 1)
}

// scopeCheckConcurrency bounds how many get-by-ID scope checks run at once in
// GetScopedEntityIDsBatch, so re-scoping a large batch of similarity hits does
// not open an unbounded number of connections to TheHive.
const scopeCheckConcurrency = 8

// GetScopedEntityIDsBatch reports which of the given entity IDs match the
// permission filters. It checks every ID with the same proven get-by-ID
// pipeline GetEntityIDsInScope uses (getCase/getAlert -> filter(permFilters)),
// but issues the checks CONCURRENTLY behind a bounded worker pool rather than
// serially — so re-scoping N similarity hits costs ~one round-trip of latency,
// not N (DL-5764).
//
// This was originally a single list query that combined the IDs with _or and
// intersected them with the permission filters:
//
//	listCase -> filter(_and[ permFilters, _or[ {_id:h1}, ... ] ])
//
// That pattern proved unreliable against real TheHive: a list operation does
// not honour an _id equality filter the way a get-by-ID lookup does, so the
// batch result disagreed with the proven get-by-ID path (verified against a
// live container by TestGetScopedEntityIDsBatchHonorsIDFilter). Because this
// guards a TLP:RED similarity-expansion leak path, correctness wins: we fan out
// the proven per-ID check and recover the latency with bounded concurrency
// instead of relying on the unsupported list filter.
//
// With no filters every ID is trivially in scope.
func GetScopedEntityIDsBatch(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]interface{}) (map[string]bool, error) {
	return scopedEntityIDsBatch(ctx, entityType, entityIDs, permFilters, scopeCheckConcurrency)
}

// scopedEntityIDsBatch is the shared implementation behind both
// GetEntityIDsInScope and GetScopedEntityIDsBatch. It checks every ID with the
// proven get-by-ID pipeline (getCase/getAlert -> filter(permFilters)), fanning
// the checks out behind a bounded worker pool. concurrency caps how many checks
// run at once: GetEntityIDsInScope passes 1 (serial), GetScopedEntityIDsBatch
// passes scopeCheckConcurrency. The result is keyed by ID, so it is independent
// of goroutine scheduling and identical regardless of concurrency.
func scopedEntityIDsBatch(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]interface{}, concurrency int) (map[string]bool, error) {
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
		return nil, fmt.Errorf("cannot verify scope: missing entity type")
	}

	filterOp := scopeFilterOperation(permFilters)

	// Derive a cancelable context so that, once one check fails (or the caller's
	// ctx is cancelled mid-batch), in-flight workers abort their queries early
	// and the dispatch loop stops issuing the remaining checks instead of firing
	// them all for a result that will be discarded.
	qctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		once     sync.Once
		firstID  string
		firstErr error
	)
	// Clamp: a zero or negative concurrency would make a 0-capacity semaphore
	// and deadlock on the first send. Treat anything < 1 as serial.
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)

	dispatchedAll := true
	for _, entityID := range entityIDs {
		// Stop dispatching once the parent ctx is cancelled or our own cancel()
		// has fired after the first error; the partial map is discarded below.
		if qctx.Err() != nil {
			dispatchedAll = false
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(entityID string) {
			defer wg.Done()
			defer func() { <-sem }()

			getOp := map[string]interface{}{
				"_name":    getOpName,
				"idOrName": entityID,
			}
			matched, err := executeScopeQuery(qctx, []thehive.InputQueryNamedOperation{
				thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&getOp),
				thehive.MapmapOfStringAnyAsInputQueryNamedOperation(&filterOp),
			})
			if err != nil {
				once.Do(func() {
					firstID = entityID
					firstErr = err
					cancel()
				})
				return
			}

			mu.Lock()
			inScope[entityID] = matched
			mu.Unlock()
		}(entityID)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, fmt.Errorf("failed to verify %s %s against permission filters: %w", entityType, firstID, firstErr)
	}
	// If the dispatch loop was cut short by a caller-ctx cancellation, the map
	// is partial; surface that as an error rather than returning a
	// silently-truncated result that looks complete. A loop that dispatched and
	// checked every ID (dispatchedAll, firstErr == nil) yields a complete map,
	// which is returned even if ctx was cancelled after the last check finished.
	if !dispatchedAll {
		return nil, fmt.Errorf("scope verification cancelled: %w", ctx.Err())
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
