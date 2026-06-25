package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *SearchTool) Handle(ctx context.Context, req mcp.CallToolRequest, params SearchEntitiesParams) (SearchEntitiesResult, error) {
	// The caller (the model) supplies the TheHive filter DSL directly in
	// params.Filters. There is no inner-LLM translation step — the filter is
	// applied as-is, after merging any permission-scoping filters.
	rawFilters := params.Filters

	// Apply permission filters
	perms, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return SearchEntitiesResult{}, tools.NewToolError("failed to get permissions").Cause(err)
	}
	permFilters := perms.GetToolFilters(t.Name())
	if len(permFilters) > 0 {
		merged, filtersApplied := permissions.MergeFilters(rawFilters, permFilters)
		if filtersApplied {
			slog.Info("Merged permission filters into search filters", "entityType", params.EntityType)
		} else {
			slog.Info("No permission filters applied to search filters", "entityType", params.EntityType)
		}
		rawFilters = merged
	}

	// Build TheHive query
	hiveQuery, err := t.buildHiveQuery(params, rawFilters)
	if err != nil {
		return SearchEntitiesResult{}, tools.NewToolError("failed to build TheHive query").Cause(err).
			Hint("This may be due to unsupported field names or filter combinations. Consult hive://schema/"+params.EntityType+" for valid fields and hive://schema/filter for the operator grammar.").
			Schema(params.EntityType, "")
	}

	// Execute query. On failure, return an actionable error so the calling model
	// can correct the filter and call again — no inner retry loop.
	results, err := t.executeQuery(ctx, hiveQuery, params.EntityType)
	if err != nil {
		return SearchEntitiesResult{}, tools.NewToolError("failed to execute search query").Cause(err).
			Hint("The filter likely references a non-existent field or an invalid value/operator. Consult hive://schema/"+params.EntityType+" for valid fields and hive://schema/filter for the operator grammar, then retry with corrected filters.").
			Schema(params.EntityType, "")
	}

	// Skip additional queries for count-only requests
	if !params.Count {
		results, err = utils.ExpandEntitiesWithQueries(ctx, params.EntityType, results, params.AdditionalQueries, permFilters)
		if err != nil {
			return SearchEntitiesResult{}, tools.NewToolError("failed to perform additional queries").Cause(err)
		}
	}

	// Process and format results
	return NewSearchEntitiesResult(results, params, rawFilters)
}

// Query building
func (t *SearchTool) buildHiveQuery(params SearchEntitiesParams, rawFilters map[string]interface{}) (thehive.InputQuery, error) {

	// Build operations
	listOp := t.buildListOperation(params.EntityType)

	// Exclude unneeded fields
	excludedFields := t.getExcludedFields(params.EntityType, params.ExtraColumns, params.ExtraData)

	query := []thehive.InputQueryNamedOperation{
		thehive.InputQueryGenericOperationAsInputQueryNamedOperation(listOp),
	}

	// Only apply a filter operation when filters were provided. An empty filter
	// means "match all entities" (within the limit).
	if len(rawFilters) > 0 {
		filterOp := t.buildFilterOperation(rawFilters)
		query = append(query, thehive.MapmapOfStringAnyAsInputQueryNamedOperation(filterOp))
	}

	if params.Count {
		countOp := thehive.NewInputQueryGenericOperation("count")
		query = append(query, thehive.InputQueryGenericOperationAsInputQueryNamedOperation(countOp))
	} else {
		sortOp := t.buildSortOperation(params.SortBy, params.SortOrder)
		pageOp := t.buildPagingOperation(params.Limit, params.ExtraData)
		query = append(query,
			thehive.InputQuerySortOperationAsInputQueryNamedOperation(sortOp),
			thehive.InputQueryPagingOperationAsInputQueryNamedOperation(pageOp),
		)
	}

	hiveQuery := thehive.InputQuery{
		Query:         query,
		ExcludeFields: excludedFields,
	}

	// Debug logging
	if queryJSON, err := json.MarshalIndent(hiveQuery, "", "  "); err == nil {
		slog.Info("Built TheHive query", "json", string(queryJSON))
	}

	return hiveQuery, nil
}

func (t *SearchTool) buildListOperation(entityType string) *thehive.InputQueryGenericOperation {
	return thehive.NewInputQueryGenericOperation(utils.ListOperationName(entityType))
}

func (t *SearchTool) buildFilterOperation(filters map[string]interface{}) *map[string]interface{} {
	// Create a shallow copy to avoid modifying the original filters
	filtersCopy := make(map[string]interface{})
	for k, v := range filters {
		filtersCopy[k] = v
	}

	parsedFilters := utils.TranslateDatesToTimestamps(filtersCopy)
	parsedFilters["_name"] = "filter"
	return &parsedFilters
}

func (t *SearchTool) buildSortOperation(sortBy, sortOrder string) *thehive.InputQuerySortOperation {
	sortOp := thehive.NewInputQuerySortOperation("sort")
	sortFields := []map[string]interface{}{
		{sortBy: sortOrder},
	}
	sortOp.SetFields(sortFields)
	return sortOp
}

func (t *SearchTool) buildPagingOperation(limit int, extraData []string) *thehive.InputQueryPagingOperation {
	query := thehive.NewInputQueryPagingOperation(0, int32(limit), "page") // #nosec G115 -- limit is validated before reaching here
	query.SetExtraData(extraData)
	return query
}

// Query execution

func (t *SearchTool) executeQuery(ctx context.Context, hiveQuery thehive.InputQuery, entityType string) ([]map[string]interface{}, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client: %w. Check your authentication and connection settings", err)
	}

	results, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to search %ss: %v. Check that you have permissions to view %ss. API response: %v", entityType, err, entityType, resp)
	}

	// Handle count queries - they return a number instead of an array
	if countValue, ok := results.(float64); ok {
		// For count queries, create a special entry to indicate the count
		countResult := map[string]interface{}{
			"_count": countValue,
		}
		return []map[string]interface{}{countResult}, nil
	}

	// Handle regular queries - they return an array of entities
	resultsInterface, ok := results.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result type from TheHive API. Expected []interface{} or float64 but got %T: %v", results, results)
	}

	// Convert each interface{} to map[string]interface{}
	resultsSlice := make([]map[string]interface{}, len(resultsInterface))
	for i, item := range resultsInterface {
		mapItem, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected item type in results. Expected map[string]interface{} but got %T at index %d: %v", item, i, item)
		}
		resultsSlice[i] = mapItem
	}

	slog.Debug("Query executed", "type", entityType, "count", len(resultsSlice))

	return resultsSlice, nil
}

// Helper methods
func (t *SearchTool) getExcludedFields(entityType string, keptColumns []string, extraData []string) []string {
	var baseModel any
	switch entityType {
	case types.EntityTypeAlert:
		baseModel = thehive.OutputAlert{}
	case types.EntityTypeCase:
		baseModel = thehive.OutputCase{}
	case types.EntityTypeTask:
		baseModel = thehive.OutputTask{}
	case types.EntityTypeObservable:
		baseModel = thehive.OutputObservable{}
	case types.EntityTypeProcedure:
		baseModel = thehive.OutputProcedure{}
	case types.EntityTypePattern:
		baseModel = thehive.OutputPattern{}
	case types.EntityTypeCaseTemplate:
		baseModel = thehive.OutputCaseTemplate{}
	case types.EntityTypePage:
		baseModel = thehive.OutputPage{}
	default:
		return []string{}
	}

	allFields := utils.GetJSONFields(baseModel)
	excludeFields := make([]string, 0)

	// System fields that must never be excluded from search results.
	// _id is required by additional queries to fetch related entities.
	systemFields := []string{"_id"}

	for _, field := range allFields {
		if !slices.Contains(keptColumns, field) {
			if slices.Contains(systemFields, field) {
				continue
			}
			if field == "extraData" && len(extraData) > 0 {
				continue
			}
			excludeFields = append(excludeFields, field)
		}
	}
	return excludeFields
}
