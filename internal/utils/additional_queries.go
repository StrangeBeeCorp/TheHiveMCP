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

// QueryFunc defines a function that queries data for a single entity
type QueryFunc func(ctx context.Context, client *thehive.APIClient, entityID string) ([]map[string]any, error)

// QueryDescriptor declares everything the expansion pipeline needs to know
// about one additional query: how to fetch it, what entity type its results
// are, whether those results are INDEPENDENT top-level entities, and which
// match-context meta fields to surface alongside the entity.
//
// ResultsAreIndependent is the security-critical property. A query is
// "independent" when its results are top-level entities returned in their own
// right (e.g. the similarity engine's similar cases/alerts) rather than
// children of an already-scoped parent. TheHive applies its own .visible org/
// profile filter to such results but NOT the MCP-level permFilters, so each hit
// must be re-checked against permFilters before it is surfaced — otherwise a hit
// the analyst could never reach via a direct search would leak through expansion
// (DL-6004). Child queries (tasks, observables, ...) traverse from a parent that
// ExpandEntitiesWithQueries already scoped, so they need no re-check.
//
// Modelling this as a per-query property (rather than an out-of-band set lookup)
// makes re-scoping a DECLARED attribute of each query: a newly added independent
// query that forgets to set ResultsAreIndependent: true is caught by review at
// the registry — and by TestEveryRegisteredQueryDeclaresScopeIntent — rather
// than silently bypassing the scope re-check and failing open.
type QueryDescriptor struct {
	// Func fetches the query's results for a single parent entity.
	Func QueryFunc
	// EntityType is the entity type of the RESULTS (e.g. similarCases returns
	// cases). Used to project default fields and to scope independent results.
	EntityType string
	// ResultsAreIndependent marks results that must be re-scoped against the
	// MCP permFilters because they are not children of an already-scoped parent.
	ResultsAreIndependent bool
	// MetaFields are match-context fields (e.g. similarObservableCount) carried
	// FLAT alongside the entity by the *Light ops and surfaced to the LLM so it
	// has match context without follow-up fetches. Empty for queries with none.
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

// queryRegistry maps entity types to their available queries. Each descriptor
// is the single source of truth for that query's fetch func, result entity
// type, scope-independence, and surfaced meta fields.
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

// projectFields copies the default entity fields from item, plus — when
// includeMeta is set — the given match-context meta fields, into a fresh map.
// The *Light similarity ops emit a FLAT object — {...richEntity...,
// "similarObservableCount": N, "linksCount": M} — so the entity and its meta
// share one source. Non-similarity results have no meta to lift, so callers
// pass includeMeta=false.
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

// filterSimilarityHitsByScope keeps only the similarity hits whose resolvable
// top-level _id is marked in-scope by a precomputed scope map. A hit with no
// resolvable _id is DROPPED — it cannot be scope-checked, so surfacing it would
// risk leaking an out-of-scope entity. This is the redistribution half of the
// old scopeSimilarityResults: the scope check itself is now a single batch call
// per target entity type made across ALL parents (see ExpandEntitiesWithQueries),
// so this helper does no I/O. *Light hits are flat, so _id is top-level.
//
// A hit dropped because its _id is UNRESOLVABLE (not a top-level string) is the
// fail-closed branch that is otherwise silent: the drop is correct, but if a
// future TheHive version nests or retypes _id in the *Light output, EVERY hit
// would resolve to "" → all dropped → empty similarity results, with no error
// surfaced. So each such drop is logged at Debug with the query name, target
// type, and the hit's top-level keys — keys only, never values, to avoid leaking
// entity data — plus one aggregate Debug line carrying droppedCount/totalHits.
// This turns a silent shape regression into a diagnosable one. Hits dropped
// merely for being out-of-scope are expected and are NOT logged (they would be
// noisy and carry no regression signal). queryName/targetType are passed for the
// log context only; the scope decision is wholly in inScope.
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

// similarityHitID extracts the top-level _id of a flat *Light similarity hit.
func similarityHitID(item map[string]any) string {
	if id, ok := item[fieldID].(string); ok {
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
	entities []map[string]any,
	additionalQueries []string,
	permFilters map[string]any,
) ([]map[string]any, error) {
	// Nothing to expand when no additional queries were requested.
	if len(additionalQueries) == 0 {
		return entities, nil
	}

	hiveClient, err := GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client: %w", err)
	}

	// Get query config for this entity type
	queryConfig, exists := queryRegistry[entityType]
	if !exists {
		return nil, fmt.Errorf("additional queries not supported for entity type: %s", entityType)
	}

	// Validate all queries before executing
	for _, queryName := range additionalQueries {
		if _, supported := queryConfig[queryName]; !supported {
			return nil, fmt.Errorf("unsupported additional query '%s' for entity type '%s'", queryName, entityType)
		}
	}

	// Deny expansion of any parent entity the permission filters exclude
	// (DL-6004). Search results are already scoped server-side; this guards
	// against any caller passing unscoped entity IDs.
	if len(permFilters) > 0 {
		entityIDs := make([]string, 0, len(entities))
		for i, entity := range entities {
			entityID, ok := entity[fieldID].(string)
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

	// Expansion runs in two passes so that similarity hits are scope-checked in
	// ONE batch per target entity type across ALL parents, instead of one batch
	// per parent. Similarity queries (similarCases/similarAlerts) return
	// independent top-level entities straight from TheHive's similarity engine,
	// not children of the parent. TheHive applies its own .visible org/profile
	// filter to them, but NOT the MCP-level permFilters (e.g. tlp<=2) — those
	// are never sent on the similarity query. So each hit must be re-checked
	// against permFilters before it is surfaced, otherwise a hit the analyst
	// could never reach via a direct search would leak through expansion
	// (DL-6004). The per-parent query execution below stays serial; only the
	// scope re-check is unified into a cross-parent batch (DL-5764).

	// Pass 1: run every query, stash its raw rows, and collect the union of
	// similarity hit _ids per target entity type (similarCases -> case,
	// similarAlerts -> alert). A set dedups an _id two parents share, so it is
	// scope-checked once rather than once per parent.
	rawResults := make([]map[string][]map[string]any, len(entities))
	idsByType := make(map[string]map[string]struct{})

	for i, entity := range entities {
		entityID, ok := entity[fieldID].(string)
		if !ok {
			return nil, fmt.Errorf("entity at index %d missing _id field", i)
		}

		rawResults[i] = make(map[string][]map[string]any, len(additionalQueries))

		for _, queryName := range additionalQueries {
			descriptor := queryConfig[queryName]

			data, err := descriptor.Func(ctx, hiveClient, entityID)
			if err != nil {
				return nil, fmt.Errorf("failed to get %s for %s ID %s: %w", queryName, entityType, entityID, err)
			}

			rawResults[i][queryName] = data

			// Only INDEPENDENT queries need a scope re-check, and only when
			// permission filters are configured. Everything else passes through.
			// Whether re-scoping applies is the descriptor's declared property
			// (ResultsAreIndependent), not a hardcoded query-name set — so a new
			// independent query is re-scoped as soon as it declares it, and a
			// query that forgets to declare it is caught by review at the registry
			// rather than silently leaking.
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

	// Pass 1.5: one batch scope call per target entity type over the union of
	// every parent's hits. Fail closed — return on the first error before any
	// hit is surfaced.
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

	// Pass 2: drop out-of-scope similarity hits using the precomputed verdicts,
	// then project the default fields and attach the result to each parent.
	for i, entity := range entities {
		for _, queryName := range additionalQueries {
			descriptor := queryConfig[queryName]
			data := rawResults[i][queryName]

			// Re-scoping applies only to INDEPENDENT queries (the descriptor's
			// declared property), using the precomputed cross-parent verdicts.
			if descriptor.ResultsAreIndependent && len(permFilters) > 0 {
				data = filterSimilarityHitsByScope(ctx, queryName, descriptor.EntityType, data, scopeByType[descriptor.EntityType])
			}

			filteredData, err := filterAdditionalQueryResults(data, descriptor)
			if err != nil {
				return nil, fmt.Errorf("failed to filter additional query results for %s ID %s: %w", entityType, entity[fieldID], err)
			}

			entity[queryName] = filteredData
		}
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
