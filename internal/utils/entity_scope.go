package utils

import (
	"context"
	"fmt"
	"sync"
	"unicode"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
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
// type, checked server-side per ID. With no filters every ID is in scope.
// See GetScopedEntityIDsBatch for why this is per-ID, not one list query.
func GetEntityIDsInScope(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]interface{}) (map[string]bool, error) {
	return scopedEntityIDsBatch(ctx, entityType, entityIDs, permFilters, 1)
}

// scopeCheckConcurrency bounds concurrent get-by-ID checks so re-scoping a large
// batch does not open unbounded connections to TheHive.
const scopeCheckConcurrency = 8

// GetScopedEntityIDsBatch is GetEntityIDsInScope behind a bounded worker pool,
// so N hits cost ~one round-trip, not N (DL-5764).
//
// Per-ID, not one list query with _or of the IDs: a list op does not honour an
// _id equality filter the way get-by-ID does, so the batch disagreed with the
// proven path (TestGetScopedEntityIDsBatchHonorsIDFilter). Guards a TLP:RED leak.
func GetScopedEntityIDsBatch(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]interface{}) (map[string]bool, error) {
	return scopedEntityIDsBatch(ctx, entityType, entityIDs, permFilters, scopeCheckConcurrency)
}

// scopedEntityIDsBatch backs both public entry points via a get-by-ID ->
// filter(permFilters) pipeline; the ID-keyed result is identical at any concurrency.
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

	// Cancelable so the first error (or caller-ctx cancel) aborts in-flight
	// workers and stops dispatching checks for a result that will be discarded.
	qctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		once     sync.Once
		firstID  string
		firstErr error
	)
	// concurrency < 1 would make a 0-capacity semaphore and deadlock on first send.
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)

	dispatchedAll := true
	for _, entityID := range entityIDs {
		// Stop on ctx cancel or first-error cancel(); partial map discarded below.
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
	// Error out rather than return a truncated scope map that looks complete.
	if !dispatchedAll {
		return nil, fmt.Errorf("scope verification cancelled: %w", ctx.Err())
	}
	return inScope, nil
}

// IsJobObservableInScope reports whether a Cortex job's target observable matches
// permFilters, resolved server-side (getJob -> observable -> filter). No filters:
// every job is in scope.
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

func scopeFilterOperation(permFilters map[string]interface{}) map[string]interface{} {
	filterOp := make(map[string]interface{}, len(permFilters)+1)
	for k, v := range permFilters {
		filterOp[k] = v
	}
	filterOp = NormalizeFilterKeys(filterOp)
	filterOp = TranslateDatesToTimestamps(filterOp)
	filterOp["_name"] = "filter"
	return filterOp
}

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

// IsEntityInScope is the single-ID form of GetEntityIDsInScope.
func IsEntityInScope(ctx context.Context, entityType, entityID string, permFilters map[string]interface{}) (bool, error) {
	inScope, err := GetEntityIDsInScope(ctx, entityType, []string{entityID}, permFilters)
	if err != nil {
		return false, err
	}
	return inScope[entityID], nil
}
