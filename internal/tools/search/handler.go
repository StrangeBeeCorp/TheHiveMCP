package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// Handle executes an entity search: it applies permission filters, builds and
// runs the TheHive query, optionally enriches results with additional queries,
// and formats them into an EntitiesResult.
func (t *Tool) Handle(ctx context.Context, req mcp.CallToolRequest, params EntitiesParams) (EntitiesResult, error) {
	// The caller (the model) supplies the TheHive filter DSL directly in
	// params.Filters. There is no inner-LLM translation step — the filter is
	// applied as-is, after merging any permission-scoping filters.
	// Non-nil map so echoed rawFilters is always an object ({} = match-all), not JSON null.
	rawFilters := params.Filters
	if rawFilters == nil {
		rawFilters = map[string]any{}
	}

	// Legacy "query" param removed in favor of "filters"; warn so ignoring it isn't mistaken for a bug.
	if _, ok := req.GetArguments()["query"]; ok {
		slog.Warn("ignoring removed 'query' parameter; build a filter with the TheHive DSL and pass it in 'filters' (see hive://docs/overview/filter-dsl)", "entityType", params.EntityType)
	}

	perms, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return EntitiesResult{}, tools.NewToolError("failed to get permissions").Cause(err)
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

	hiveQuery := t.buildHiveQuery(params, rawFilters)

	results, err := t.executeQuery(ctx, hiveQuery, params.EntityType)
	if err != nil {
		return EntitiesResult{}, tools.NewToolError("failed to execute search query").Cause(err).
			Hint("The filter likely references a non-existent field or an invalid value/operator. Consult hive://schema/"+params.EntityType+" for valid fields and hive://schema/filter for the operator grammar, then retry with corrected filters.").
			Schema(params.EntityType, "")
	}

	if !params.Count {
		results, err = utils.ExpandEntitiesWithQueries(ctx, params.EntityType, results, params.AdditionalQueries, permFilters)
		if err != nil {
			return EntitiesResult{}, tools.NewToolError("failed to perform additional queries").Cause(err)
		}
	}

	return NewSearchEntitiesResult(results, params, rawFilters)
}

func (t *Tool) buildHiveQuery(params EntitiesParams, rawFilters map[string]any) thehive.InputQuery {
	listOp := t.buildListOperation(params.EntityType)
	excludedFields := t.getExcludedFields(params.EntityType, params.ExtraColumns, params.ExtraData)

	query := []thehive.InputQueryNamedOperation{
		thehive.InputQueryGenericOperationAsInputQueryNamedOperation(listOp),
	}

	// Empty filter means match-all, so only add a filter op when filters exist.
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

	queryJSON, err := json.MarshalIndent(hiveQuery, "", "  ")
	if err == nil {
		slog.Info("Built TheHive query", "json", string(queryJSON))
	}

	return hiveQuery
}

func (t *Tool) buildListOperation(entityType string) *thehive.InputQueryGenericOperation {
	return thehive.NewInputQueryGenericOperation(utils.ListOperationName(entityType))
}

func (t *Tool) buildFilterOperation(filters map[string]any) *map[string]any {
	// Shallow copy so we don't mutate the caller's filters.
	filtersCopy := make(map[string]any)
	maps.Copy(filtersCopy, filters)

	// Repair malformed keys (over-quoting/whitespace from weaker models) before
	// the date pass so it and TheHive see valid keys.
	normalizedFilters := utils.NormalizeFilterKeys(filtersCopy)
	parsedFilters := utils.TranslateDatesToTimestamps(normalizedFilters)
	parsedFilters["_name"] = "filter"

	return &parsedFilters
}

func (t *Tool) buildSortOperation(sortBy, sortOrder string) *thehive.InputQuerySortOperation {
	sortOp := thehive.NewInputQuerySortOperation("sort")
	sortFields := []map[string]any{
		{sortBy: sortOrder},
	}
	sortOp.SetFields(sortFields)

	return sortOp
}

func (t *Tool) buildPagingOperation(limit int, extraData []string) *thehive.InputQueryPagingOperation {
	query := thehive.NewInputQueryPagingOperation(0, int32(limit), "page") // #nosec G115 -- limit is validated before reaching here
	query.SetExtraData(extraData)

	return query
}

func (t *Tool) executeQuery(ctx context.Context, hiveQuery thehive.InputQuery, entityType string) ([]map[string]any, error) {
	hiveClient, err := utils.GetHiveClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get TheHive client: %w. Check your authentication and connection settings", err)
	}

	results, resp, err := hiveClient.QueryAndExportAPI.QueryAPI(ctx).InputQuery(hiveQuery).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to search %ss: %w. Check that you have permissions to view %ss. API response: %v", entityType, err, entityType, resp)
	}

	// Count queries return a bare number, not an array.
	if countValue, ok := results.(float64); ok {
		countResult := map[string]any{
			"_count": countValue,
		}

		return []map[string]any{countResult}, nil
	}

	resultsInterface, ok := results.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from TheHive API. Expected []interface{} or float64 but got %T: %v", results, results)
	}

	resultsSlice := make([]map[string]any, len(resultsInterface))
	for i, item := range resultsInterface {
		mapItem, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unexpected item type in results. Expected map[string]interface{} but got %T at index %d: %v", item, i, item)
		}

		resultsSlice[i] = mapItem
	}

	slog.Debug("Query executed", "type", entityType, "count", len(resultsSlice))

	return resultsSlice, nil
}

// baseModels holds the output model each entity type projects onto, used to work
// out which fields to exclude from a result. An entity type absent from the map
// gets no exclusions.
var baseModels = map[string]any{
	types.EntityTypeAlert:        thehive.OutputAlert{},
	types.EntityTypeCase:         thehive.OutputCase{},
	types.EntityTypeTask:         thehive.OutputTask{},
	types.EntityTypeObservable:   thehive.OutputObservable{},
	types.EntityTypeProcedure:    thehive.OutputProcedure{},
	types.EntityTypePattern:      thehive.OutputPattern{},
	types.EntityTypeCaseTemplate: thehive.OutputCaseTemplate{},
	types.EntityTypePage:         thehive.OutputPage{},
	types.EntityTypeJob:          thehive.OutputJob{},
	types.EntityTypeAction:       thehive.OutputAction{},
}

func (t *Tool) getExcludedFields(entityType string, keptColumns []string, extraData []string) []string {
	baseModel, known := baseModels[entityType]
	if !known {
		return []string{}
	}

	allFields := utils.GetJSONFields(baseModel)
	excludeFields := make([]string, 0)

	// Never exclude _id: additional queries need it to fetch related entities.
	systemFields := []string{fieldID}

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
