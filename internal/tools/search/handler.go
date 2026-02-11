package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"unicode"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/prompts"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

const maxSearchRetries = 3

func (t *SearchTool) Handle(ctx context.Context, req mcp.CallToolRequest, params SearchEntitiesParams) (SearchEntitiesResult, error) {
	additionalMessages := []mcp.PromptMessage{}
	for attempt := 1; attempt <= maxSearchRetries; attempt++ {
		// 3. Get filters from natural language query
		filters, err := t.parseQuery(ctx, params, additionalMessages)
		if err != nil {
			return SearchEntitiesResult{}, tools.NewToolError("failed to parse natural language query").Cause(err).
				Hint("Try rephrasing your query or check that the entity type supports the fields you're searching for").
				Schema(params.EntityType, "")
		}

		// 4. Apply permission filters
		perms, err := utils.GetPermissionsFromContext(ctx)
		if err != nil {
			return SearchEntitiesResult{}, tools.NewToolError("failed to get permissions").Cause(err)
		}
		permFilters := perms.GetToolFilters(t.Name())
		if len(permFilters) > 0 {
			rawFilters, filtersApplied := permissions.MergeFilters(filters.RawFilters, permFilters)
			if filtersApplied {
				slog.Info("Merged permission filters into search filters", "entityType", params.EntityType)
			} else {
				slog.Info("No permission filters applied to search filters", "entityType", params.EntityType)
			}
			filters.RawFilters = rawFilters
		}

		// 5. Build TheHive query
		hiveQuery, err := t.buildHiveQuery(params, filters)
		if err != nil {
			return SearchEntitiesResult{}, tools.NewToolError("failed to build TheHive query").Cause(err).
				Hint("This may be due to unsupported field names or filter combinations").
				Schema(params.EntityType, "")
		}

		// 6. Execute query
		results, err := t.executeQuery(ctx, hiveQuery, params.EntityType)
		if err != nil {
			slog.Warn("Search attempt failed, retrying", "attempt", attempt, "error", err)
			additionalMessages, err = expandAdditionalMessages(additionalMessages, filters, err)
			if err != nil {
				return SearchEntitiesResult{}, tools.NewToolError("failed to expand messages for retry").Cause(err)
			}
			continue
		}

		// Skip additional queries for count-only requests
		if !params.Count {
			results, err = utils.ExpandEntitiesWithQueries(ctx, params.EntityType, results, filters.AdditionalQueries)
			if err != nil {
				return SearchEntitiesResult{}, tools.NewToolError("failed to perform additional queries").Cause(err)
			}
		}

		// 7. Process and format results
		return NewSearchEntitiesResult(results, params, filters.RawFilters)
	}

	return SearchEntitiesResult{}, tools.NewToolError("maximum search retries exceeded").
		Hint("The query could not be translated to valid TheHive filters").
		Hint("Try simplifying your search criteria or using more specific field names")
}

func expandAdditionalMessages(original []mcp.PromptMessage, filters *FilterResult, execErr error) ([]mcp.PromptMessage, error) {
	filtersJSON, err := json.MarshalIndent(filters.RawFilters, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal filters for retry: %w", err)
	}
	messages := append(original, mcp.PromptMessage{
		Role:    mcp.RoleAssistant,
		Content: mcp.NewTextContent(string(filtersJSON)),
	})
	messages = append(messages, mcp.PromptMessage{
		Role:    mcp.RoleUser,
		Content: mcp.NewTextContent(fmt.Sprintf("The previous filters resulted in an error: %v. Please adjust the filters accordingly.", execErr)),
	})
	return messages, nil
}

func (t *SearchTool) parseQuery(ctx context.Context, params SearchEntitiesParams, additionalMessages []mcp.PromptMessage) (*FilterResult, error) {
	query, err := json.MarshalIndent(params, "", "  ")
	slog.Debug("Search parameters for query parsing", "json", string(query))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search params: %w", err)
	}
	prompt, err := prompts.GetBuildFiltersPrompt(ctx, string(query), params.EntityType)
	if err != nil {
		return nil, fmt.Errorf("failed to get search filter prompt: %w. This may indicate missing resources or system configuration issues", err)
	}
	messages := append(prompt.Messages, additionalMessages...)
	var filterResult FilterResult
	err = utils.GetModelCompletion(ctx, messages, &filterResult)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI model completion for query parsing: %w. Check that the AI service is available and configured correctly", err)
	}

	slog.Info("Parsed natural language query", "query", query, "filters", filterResult.RawFilters)

	return &filterResult, nil
}

// Query building
func (t *SearchTool) buildHiveQuery(params SearchEntitiesParams, filters *FilterResult) (thehive.InputQuery, error) {

	// Build operations
	listOp := t.buildListOperation(params.EntityType)
	filterOp := t.buildFilterOperation(filters.RawFilters)

	// Exclude unneeded fields
	excludedFields := t.getExcludedFields(params.EntityType, filters.KeptColumns, filters.ExtraData)

	query := []thehive.InputQueryNamedOperation{
		thehive.InputQueryGenericOperationAsInputQueryNamedOperation(listOp),
		thehive.MapmapOfStringAnyAsInputQueryNamedOperation(filterOp),
	}

	if params.Count {
		countOp := thehive.NewInputQueryGenericOperation("count")
		query = append(query, thehive.InputQueryGenericOperationAsInputQueryNamedOperation(countOp))
	} else {
		sortOp := t.buildSortOperation(filters.SortBy, filters.SortOrder)
		pageOp := t.buildPagingOperation(filters.NumResults, filters.ExtraData)
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
	catpitalizedEntityType := string(unicode.ToUpper(rune(entityType[0]))) + entityType[1:]
	operationName := fmt.Sprintf("list%s", catpitalizedEntityType)
	return thehive.NewInputQueryGenericOperation(operationName)
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
	query := thehive.NewInputQueryPagingOperation(0, int32(limit), "page")
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
	default:
		return []string{}
	}

	allFields := utils.GetJSONFields(baseModel)
	excludeFields := make([]string, 0)

	for _, field := range allFields {
		if !slices.Contains(keptColumns, field) {
			if field == "extraData" && len(extraData) > 0 {
				continue
			}
			excludeFields = append(excludeFields, field)
		}
	}
	return excludeFields
}
