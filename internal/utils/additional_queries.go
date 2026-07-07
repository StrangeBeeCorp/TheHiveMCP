// Package utils provides shared helpers for the MCP server: TheHive query
// building, entity-scope permission checks, filter normalization, result
// date/untrusted-data processing, and the elicitation HTTP transport.
package utils

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

// QueryFunc queries data for a single entity.
type QueryFunc func(ctx context.Context, client *thehive.APIClient, entityID string) ([]map[string]any, error)

// QueryDescriptor is the single source of truth for one additional query.
//
// ResultsAreIndependent is security-critical: results TheHive returns as
// top-level entities (e.g. similar cases/alerts) get its .visible org/profile
// filter but NOT the MCP permFilters, so each hit must be re-checked against
// permFilters before surfacing or it leaks past the analyst's search scope
// (DL-6004). Child queries traverse from an already-scoped parent, so no
// re-check. Declaring it per-query (vs an out-of-band set) means a new
// independent query that forgets ResultsAreIndependent:true fails review at the
// registry / TestEveryRegisteredQueryDeclaresScopeIntent instead of failing open.
type QueryDescriptor struct {
	Func QueryFunc
	// EntityType is the type of the RESULTS (e.g. similarCases returns cases),
	// used to project default fields and scope independent results.
	EntityType string
	// ResultsAreIndependent: re-scope these against permFilters (see above).
	ResultsAreIndependent bool
	// MetaFields are match-context fields (e.g. similarObservableCount) carried
	// FLAT alongside the entity by the *Light ops. Empty for queries with none.
	MetaFields []string
}

// EntityQueryConfig maps query names to their descriptors.
type EntityQueryConfig map[string]QueryDescriptor

// similarityMetaFields are preserved alongside the entity to give the LLM
// observable-match context without requiring follow-up fetches. The *Light
// similarity ops emit a single FLAT object per hit — entity fields and meta
// side by side, with no {"case"|"alert": {...}} wrapper — so these are lifted
// onto the projected entity (see projectFields). Shared by the four similarity
// descriptors below.
var similarityMetaFields = []string{"similarObservableCount", fieldObservableCount}

var queryRegistry = map[string]EntityQueryConfig{
	types.EntityTypeCase: {
		"tasks":       {Func: GetTasksFromCaseID, EntityType: types.EntityTypeTask},
		"observables": {Func: GetObservablesFromCaseID, EntityType: types.EntityTypeObservable},
		"comments":    {Func: GetCommentsFromCaseID, EntityType: types.EntityTypeComment},
		"pages":       {Func: GetPagesFromCaseID, EntityType: types.EntityTypePage},
		"attachments": {Func: GetAttachmentsFromCaseID, EntityType: types.EntityTypeAttachment},
		"procedures":  {Func: GetProceduresFromCaseID, EntityType: types.EntityTypeProcedure},
		querySimilarCases: {
			Func: GetSimilarCasesFromCaseID, EntityType: types.EntityTypeCase,
			ResultsAreIndependent: true, MetaFields: similarityMetaFields,
		},
		querySimilarAlerts: {
			Func: GetSimilarAlertsFromCaseID, EntityType: types.EntityTypeAlert,
			ResultsAreIndependent: true, MetaFields: similarityMetaFields,
		},
	},
	types.EntityTypeAlert: {
		"observables": {Func: GetObservablesFromAlertID, EntityType: types.EntityTypeObservable},
		"comments":    {Func: GetCommentsFromAlertID, EntityType: types.EntityTypeComment},
		"pages":       {Func: GetPagesFromAlertID, EntityType: types.EntityTypePage},
		"attachments": {Func: GetAttachmentsFromAlertID, EntityType: types.EntityTypeAttachment},
		"procedures":  {Func: GetProceduresFromAlertID, EntityType: types.EntityTypeProcedure},
		querySimilarCases: {
			Func: GetSimilarCasesFromAlertID, EntityType: types.EntityTypeCase,
			ResultsAreIndependent: true, MetaFields: similarityMetaFields,
		},
		querySimilarAlerts: {
			Func: GetSimilarAlertsFromAlertID, EntityType: types.EntityTypeAlert,
			ResultsAreIndependent: true, MetaFields: similarityMetaFields,
		},
	},
	types.EntityTypeTask: {
		"task-logs": {Func: GetTaskLogsFromTaskID, EntityType: types.EntityTypeTaskLog},
	},
	types.EntityTypeObservable: {
		// No additional queries supported yet
	},
	types.EntityTypeCaseTemplate: {
		// No additional queries supported yet
	},
	types.EntityTypePage: {
		// No additional queries supported yet
	},
}

func filterAdditionalQueryResults(results []map[string]any, descriptor QueryDescriptor) ([]map[string]any, error) {
	if descriptor.EntityType == "" {
		return nil, errors.New("query descriptor missing result entity type")
	}

	fields := types.DefaultFields[descriptor.EntityType]
	includeMeta := descriptor.ResultsAreIndependent

	filtered := make([]map[string]any, 0, len(results))
	for _, item := range results {
		filtered = append(filtered, projectFields(item, fields, includeMeta, descriptor.MetaFields))
	}

	return filtered, nil
}

// projectFields copies fields (plus metaFields when includeMeta) from item into
// a fresh map. *Light similarity ops emit one FLAT object per hit (entity fields
// and meta side by side, no {"case"|"alert":{...}} wrapper), so both are lifted
// from the same source.
func projectFields(item map[string]any, fields []string, includeMeta bool, metaFields []string) map[string]any {
	filteredItem := make(map[string]any)

	for _, field := range fields {
		if value, exists := item[field]; exists {
			filteredItem[field] = value
		}
	}

	if !includeMeta {
		return filteredItem
	}

	for _, metaField := range metaFields {
		if value, exists := item[metaField]; exists {
			filteredItem[metaField] = value
		}
	}

	return filteredItem
}

// filterSimilarityHitsByScope keeps only hits whose top-level _id is in the
// precomputed inScope map (no I/O; the batch scope call lives in
// ExpandEntitiesWithQueries). A hit with no resolvable _id is DROPPED fail-closed
// — surfacing it risks leaking an out-of-scope entity.
//
// Such drops log keys only (never values) at Debug so a future *Light shape
// change that nests or retypes _id is diagnosable rather than silently emptying
// results; out-of-scope drops are expected and not logged.
func filterSimilarityHitsByScope(ctx context.Context, queryName, targetType string, results []map[string]any, inScope map[string]bool) []map[string]any {
	kept := make([]map[string]any, 0, len(results))
	unresolvable := 0

	for _, item := range results {
		id := similarityHitID(item)
		if id == "" {
			unresolvable++

			slog.DebugContext(ctx, "Dropping similarity hit with no resolvable _id",
				slog.String("query", queryName),
				slog.String("targetType", targetType),
				slog.Any("hitKeys", slices.Sorted(maps.Keys(item))))

			continue
		}

		if inScope[id] {
			kept = append(kept, item)
		}
	}

	if unresolvable > 0 {
		slog.DebugContext(ctx, "Dropped similarity hits with no resolvable _id",
			slog.String("query", queryName),
			slog.String("targetType", targetType),
			slog.Int("droppedCount", unresolvable),
			slog.Int("totalHits", len(results)))
	}

	return kept
}

func similarityHitID(item map[string]any) string {
	if id, ok := item[fieldID].(string); ok {
		return id
	}

	return ""
}

// validateAdditionalQueries resolves the query config for entityType and
// verifies every requested query is supported for it.
func validateAdditionalQueries(entityType string, additionalQueries []string) (EntityQueryConfig, error) {
	queryConfig, exists := queryRegistry[entityType]
	if !exists {
		return nil, fmt.Errorf("additional queries not supported for entity type: %s", entityType)
	}

	for _, queryName := range additionalQueries {
		if _, supported := queryConfig[queryName]; !supported {
			return nil, fmt.Errorf("unsupported additional query '%s' for entity type '%s'", queryName, entityType)
		}
	}

	return queryConfig, nil
}

// collectEntityIDs extracts the _id of every entity, erroring if any is missing.
func collectEntityIDs(entities []map[string]any) ([]string, error) {
	entityIDs := make([]string, 0, len(entities))
	for i, entity := range entities {
		entityID, ok := entity[fieldID].(string)
		if !ok {
			return nil, fmt.Errorf("entity at index %d missing _id field", i)
		}

		entityIDs = append(entityIDs, entityID)
	}

	return entityIDs, nil
}

// verifyParentEntitiesInScope denies expansion of any parent the permission
// filters exclude, in case a caller passes unscoped entity IDs (DL-6004).
func verifyParentEntitiesInScope(ctx context.Context, entityType string, entityIDs []string, permFilters map[string]any) error {
	inScope, err := GetEntityIDsInScope(ctx, entityType, entityIDs, permFilters)
	if err != nil {
		return fmt.Errorf("failed to verify entity scope before expansion: %w", err)
	}

	for _, entityID := range entityIDs {
		if !inScope[entityID] {
			return fmt.Errorf("%s %s was not found or is not within the scope permitted by your permissions configuration", entityType, entityID)
		}
	}

	return nil
}

// runQueriesAndCollectHitIDs runs every requested query against every entity
// (pass 1), stashing the raw rows and collecting the deduped union of
// similarity-hit _ids per target type for the batched scope re-check.
func runQueriesAndCollectHitIDs(
	ctx context.Context,
	hiveClient *thehive.APIClient,
	entityType string,
	entities []map[string]any,
	additionalQueries []string,
	queryConfig EntityQueryConfig,
	permFilters map[string]any,
) ([]map[string][]map[string]any, map[string]map[string]struct{}, error) {
	rawResults := make([]map[string][]map[string]any, len(entities))
	idsByType := make(map[string]map[string]struct{})

	for i, entity := range entities {
		entityID, ok := entity[fieldID].(string)
		if !ok {
			return nil, nil, fmt.Errorf("entity at index %d missing _id field", i)
		}

		rawResults[i] = make(map[string][]map[string]any, len(additionalQueries))

		for _, queryName := range additionalQueries {
			descriptor := queryConfig[queryName]

			data, err := descriptor.Func(ctx, hiveClient, entityID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get %s for %s ID %s: %w", queryName, entityType, entityID, err)
			}

			rawResults[i][queryName] = data

			if !descriptor.ResultsAreIndependent || len(permFilters) == 0 {
				continue
			}

			collectHitIDs(idsByType, descriptor.EntityType, data)
		}
	}

	return rawResults, idsByType, nil
}

// collectHitIDs adds the non-empty similarity-hit _ids of data to the dedup set
// for targetType.
func collectHitIDs(idsByType map[string]map[string]struct{}, targetType string, data []map[string]any) {
	set := idsByType[targetType]
	if set == nil {
		set = make(map[string]struct{})
		idsByType[targetType] = set
	}

	for _, item := range data {
		if id := similarityHitID(item); id != "" {
			set[id] = struct{}{}
		}
	}
}

// computeScopeByType runs one batch scope call per target type (pass 1.5),
// failing closed on the first error before any hit is surfaced.
func computeScopeByType(ctx context.Context, idsByType map[string]map[string]struct{}, permFilters map[string]any) (map[string]map[string]bool, error) {
	scopeByType := make(map[string]map[string]bool, len(idsByType))
	for targetType, set := range idsByType {
		ids := make([]string, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}

		inScope, err := GetScopedEntityIDsBatch(ctx, targetType, ids, permFilters)
		if err != nil {
			return nil, fmt.Errorf("failed to verify similarity hit scope: %w", err)
		}

		scopeByType[targetType] = inScope
	}

	return scopeByType, nil
}

// attachFilteredResults drops out-of-scope hits per the precomputed verdicts,
// projects each query result, and attaches it to its parent entity (pass 2).
func attachFilteredResults(
	ctx context.Context,
	entityType string,
	entities []map[string]any,
	additionalQueries []string,
	queryConfig EntityQueryConfig,
	permFilters map[string]any,
	rawResults []map[string][]map[string]any,
	scopeByType map[string]map[string]bool,
) error {
	for i, entity := range entities {
		for _, queryName := range additionalQueries {
			descriptor := queryConfig[queryName]
			data := rawResults[i][queryName]

			if descriptor.ResultsAreIndependent && len(permFilters) > 0 {
				data = filterSimilarityHitsByScope(ctx, queryName, descriptor.EntityType, data, scopeByType[descriptor.EntityType])
			}

			filteredData, err := filterAdditionalQueryResults(data, descriptor)
			if err != nil {
				return fmt.Errorf("failed to filter additional query results for %s ID %s: %w", entityType, entity[fieldID], err)
			}

			entity[queryName] = filteredData
		}
	}

	return nil
}

// ExpandEntitiesWithQueries expands each entity with its related data inline.
// When permission filters are configured for the calling tool, every parent
// entity must be within the filtered scope before its children are fetched.
func ExpandEntitiesWithQueries(
	ctx context.Context,
	entityType string,
	entities []map[string]any,
	additionalQueries []string,
	permFilters map[string]any,
) ([]map[string]any, error) {
	if len(additionalQueries) == 0 {
		return entities, nil
	}

	hiveClient, err := GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client: %w", err)
	}

	queryConfig, err := validateAdditionalQueries(entityType, additionalQueries)
	if err != nil {
		return nil, err
	}

	// Deny expansion of any parent the permission filters exclude, in case a
	// caller passes unscoped entity IDs (DL-6004).
	if len(permFilters) > 0 {
		entityIDs, err := collectEntityIDs(entities)
		if err != nil {
			return nil, err
		}

		if err := verifyParentEntitiesInScope(ctx, entityType, entityIDs, permFilters); err != nil {
			return nil, err
		}
	}

	// Batch the independent-query re-check (see QueryDescriptor, DL-6004) in ONE
	// call per target type across ALL parents, not once per parent (DL-5764).
	// Query execution stays serial; only the re-check is batched.
	rawResults, idsByType, err := runQueriesAndCollectHitIDs(ctx, hiveClient, entityType, entities, additionalQueries, queryConfig, permFilters)
	if err != nil {
		return nil, err
	}

	scopeByType, err := computeScopeByType(ctx, idsByType, permFilters)
	if err != nil {
		return nil, err
	}

	if err := attachFilteredResults(ctx, entityType, entities, additionalQueries, queryConfig, permFilters, rawResults, scopeByType); err != nil {
		return nil, err
	}

	return entities, nil
}

// GetSupportedQueries returns the list of supported queries for an entity type
func GetSupportedQueries(entityType string) []string {
	queryConfig, exists := queryRegistry[entityType]
	if !exists {
		return nil
	}

	queries := make([]string, 0, len(queryConfig))
	for queryName := range queryConfig {
		queries = append(queries, queryName)
	}

	return queries
}

// ValidateQuery checks if a query is supported for an entity type
func ValidateQuery(entityType, queryName string) error {
	queryConfig, exists := queryRegistry[entityType]
	if !exists {
		return fmt.Errorf("entity type '%s' not supported", entityType)
	}

	if _, supported := queryConfig[queryName]; !supported {
		return fmt.Errorf("query '%s' not supported for entity type '%s'", queryName, entityType)
	}

	return nil
}
