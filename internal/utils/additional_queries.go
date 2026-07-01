package utils

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

// QueryFunc queries data for a single entity.
type QueryFunc func(ctx context.Context, client *thehive.APIClient, entityID string) ([]map[string]interface{}, error)

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

type EntityQueryConfig map[string]QueryDescriptor

var similarityMetaFields = []string{"similarObservableCount", "observableCount"}

var queryRegistry = map[string]EntityQueryConfig{
	types.EntityTypeCase: {
		"tasks":       {Func: GetTasksFromCaseID, EntityType: types.EntityTypeTask},
		"observables": {Func: GetObservablesFromCaseID, EntityType: types.EntityTypeObservable},
		"comments":    {Func: GetCommentsFromCaseID, EntityType: types.EntityTypeComment},
		"pages":       {Func: GetPagesFromCaseID, EntityType: types.EntityTypePage},
		"attachments": {Func: GetAttachmentsFromCaseID, EntityType: types.EntityTypeAttachment},
		"procedures":  {Func: GetProceduresFromCaseID, EntityType: types.EntityTypeProcedure},
		"similarCases": {
			Func: GetSimilarCasesFromCaseID, EntityType: types.EntityTypeCase,
			ResultsAreIndependent: true, MetaFields: similarityMetaFields,
		},
		"similarAlerts": {
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
		"similarCases": {
			Func: GetSimilarCasesFromAlertID, EntityType: types.EntityTypeCase,
			ResultsAreIndependent: true, MetaFields: similarityMetaFields,
		},
		"similarAlerts": {
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

func filterAdditionalQueryResults(results []map[string]interface{}, descriptor QueryDescriptor) ([]map[string]interface{}, error) {
	if descriptor.EntityType == "" {
		return nil, fmt.Errorf("query descriptor missing result entity type")
	}

	fields := types.DefaultFields[descriptor.EntityType]
	includeMeta := descriptor.ResultsAreIndependent

	filtered := make([]map[string]interface{}, 0, len(results))
	for _, item := range results {
		filtered = append(filtered, projectFields(item, fields, includeMeta, descriptor.MetaFields))
	}
	return filtered, nil
}

// projectFields copies fields (plus metaFields when includeMeta) from item into
// a fresh map. *Light similarity ops emit one FLAT object per hit (entity fields
// and meta side by side, no {"case"|"alert":{...}} wrapper), so both are lifted
// from the same source.
func projectFields(item map[string]interface{}, fields []string, includeMeta bool, metaFields []string) map[string]interface{} {
	filteredItem := make(map[string]interface{})
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
func filterSimilarityHitsByScope(ctx context.Context, queryName, targetType string, results []map[string]interface{}, inScope map[string]bool) []map[string]interface{} {
	kept := make([]map[string]interface{}, 0, len(results))
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

func similarityHitID(item map[string]interface{}) string {
	if id, ok := item["_id"].(string); ok {
		return id
	}
	return ""
}

// ExpandEntitiesWithQueries expands each entity with its related data inline.
// When permission filters are configured for the calling tool, every parent
// entity must be within the filtered scope before its children are fetched.
func ExpandEntitiesWithQueries(
	ctx context.Context,
	entityType string,
	entities []map[string]interface{},
	additionalQueries []string,
	permFilters map[string]interface{},
) ([]map[string]interface{}, error) {
	if len(additionalQueries) == 0 {
		return entities, nil
	}

	hiveClient, err := GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client: %w", err)
	}

	queryConfig, exists := queryRegistry[entityType]
	if !exists {
		return nil, fmt.Errorf("additional queries not supported for entity type: %s", entityType)
	}

	for _, queryName := range additionalQueries {
		if _, supported := queryConfig[queryName]; !supported {
			return nil, fmt.Errorf("unsupported additional query '%s' for entity type '%s'", queryName, entityType)
		}
	}

	// Deny expansion of any parent the permission filters exclude, in case a
	// caller passes unscoped entity IDs (DL-6004).
	if len(permFilters) > 0 {
		entityIDs := make([]string, 0, len(entities))
		for i, entity := range entities {
			entityID, ok := entity["_id"].(string)
			if !ok {
				return nil, fmt.Errorf("entity at index %d missing _id field", i)
			}
			entityIDs = append(entityIDs, entityID)
		}
		inScope, err := GetEntityIDsInScope(ctx, entityType, entityIDs, permFilters)
		if err != nil {
			return nil, fmt.Errorf("failed to verify entity scope before expansion: %w", err)
		}
		for _, entityID := range entityIDs {
			if !inScope[entityID] {
				return nil, fmt.Errorf("%s %s was not found or is not within the scope permitted by your permissions configuration", entityType, entityID)
			}
		}
	}

	// Batch the independent-query re-check (see QueryDescriptor, DL-6004) in ONE
	// call per target type across ALL parents, not once per parent (DL-5764).
	// Query execution stays serial; only the re-check is batched.

	// Pass 1: run every query, stash raw rows, collect the deduped union of
	// similarity hit _ids per target type.
	rawResults := make([]map[string][]map[string]interface{}, len(entities))
	idsByType := make(map[string]map[string]struct{})

	for i, entity := range entities {
		entityID, ok := entity["_id"].(string)
		if !ok {
			return nil, fmt.Errorf("entity at index %d missing _id field", i)
		}
		rawResults[i] = make(map[string][]map[string]interface{}, len(additionalQueries))

		for _, queryName := range additionalQueries {
			descriptor := queryConfig[queryName]

			data, err := descriptor.Func(ctx, hiveClient, entityID)
			if err != nil {
				return nil, fmt.Errorf("failed to get %s for %s ID %s: %w", queryName, entityType, entityID, err)
			}
			rawResults[i][queryName] = data

			if !descriptor.ResultsAreIndependent || len(permFilters) == 0 {
				continue
			}
			targetType := descriptor.EntityType
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
	}

	// Pass 1.5: one batch scope call per target type; fail closed on the first
	// error before any hit is surfaced.
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

	// Pass 2: drop out-of-scope hits per the precomputed verdicts, project, attach.
	for i := range entities {
		for _, queryName := range additionalQueries {
			descriptor := queryConfig[queryName]
			data := rawResults[i][queryName]

			if descriptor.ResultsAreIndependent && len(permFilters) > 0 {
				data = filterSimilarityHitsByScope(ctx, queryName, descriptor.EntityType, data, scopeByType[descriptor.EntityType])
			}

			filteredData, err := filterAdditionalQueryResults(data, descriptor)
			if err != nil {
				return nil, fmt.Errorf("failed to filter additional query results for %s ID %s: %w", entityType, entities[i]["_id"], err)
			}
			entities[i][queryName] = filteredData
		}
	}

	return entities, nil
}

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
