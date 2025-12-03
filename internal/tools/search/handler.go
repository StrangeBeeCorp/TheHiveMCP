package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/permissions"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/prompts"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
	"github.com/mark3labs/mcp-go/mcp"
)

const maxSearchRetries = 3

func (t *SearchTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 1. Check permissions
	perms, err := utils.GetPermissionsFromContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get permissions: %v", err)), nil
	}

	if !perms.IsToolAllowed("search-entities") {
		return mcp.NewToolResultError("search-entities tool is not permitted by your permissions configuration"), nil
	}

	// 2. Extract and validate parameters
	params, err := t.extractParams(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	additionalMessages := []mcp.PromptMessage{}
	for attempt := 1; attempt <= maxSearchRetries; attempt++ {
		// 3. Get filters from natural language query
		filters, err := t.parseQuery(ctx, params, additionalMessages)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse natural language query: %v. Try rephrasing your query or check that the entity type supports the fields you're searching for. Use get-resource 'hive://schema/%s' for available fields.", err, params.EntityType)), nil
		}

		// 4. Apply permission filters
		permFilters := perms.GetToolFilters("search-entities")
		if len(permFilters) > 0 {
			filters.RawFilters, _ = permissions.MergeFilters(filters.RawFilters, permFilters)
			slog.Info("Applied permission filters to search query", "entityType", params.EntityType)
		}

		// 5. Build TheHive query
		hiveQuery, err := t.buildHiveQuery(params, filters)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to build TheHive query: %v. This may be due to unsupported field names or filter combinations. Use get-resource 'hive://schema/%s' to see available fields.", err, params.EntityType)), nil
		}

		// 6. Execute query
		results, err := t.executeQuery(ctx, hiveQuery, params.EntityType)
		if err != nil {
			slog.Warn("Search attempt failed, retrying", "attempt", attempt, "error", err)
			additionalMessages, err = expandAdditionalMessages(additionalMessages, filters, err)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to expand messages for retry: %v", err)), nil
			}
			continue
		}
		results, err = utils.ExpandEntitiesWithQueries(ctx, params.EntityType, results, filters.AdditionalQueries)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to perform additional queries: %v", err)), nil
		}
		// 7. Process and format results
		return t.formatResults(results, params, filters.RawFilters)
	}
	return mcp.NewToolResultError("maximum search retries exceeded. The query could not be translated to valid TheHive filters. Try simplifying your search criteria or using more specific field names."), nil
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

// Parameter extraction and validation
type searchParams struct {
	EntityType        string
	Query             string
	SortBy            string
	SortOrder         string
	Limit             int
	ExtraColumns      []string
	ExtraData         []string
	AdditionalQueries []string
}

// extractParams extracts and validates parameters from the tool request
func (t *SearchTool) extractParams(req mcp.CallToolRequest) (*searchParams, error) {
	entityType := req.GetString("entity-type", "")
	if entityType == "" {
		return nil, fmt.Errorf("entity-type parameter is required. Must be one of: 'alert', 'case', 'task', 'observable'")
	}

	query := req.GetString("query", "")
	if query == "" {
		return nil, fmt.Errorf("query parameter is required. Provide a natural language description of what entities to find, e.g., 'high severity alerts from last week'")
	}

	sortBy := req.GetString("sort-by", "_createdAt")

	sortOrder := req.GetString("sort-order", "desc")

	params := &searchParams{
		EntityType:        entityType,
		Query:             query,
		SortBy:            sortBy,
		SortOrder:         sortOrder,
		Limit:             int(req.GetInt("limit", 10)),
		ExtraColumns:      req.GetStringSlice("extra-columns", []string{"_id", "title"}),
		ExtraData:         req.GetStringSlice("extra-data", []string{}),
		AdditionalQueries: req.GetStringSlice("additional-queries", []string{}),
	}

	slog.Info("SearchEntities called",
		"entityType", params.EntityType,
		"query", params.Query,
		"sortBy", params.SortBy,
		"limit", params.Limit)

	return params, nil
}

// Query parsing
type FilterResult struct {
	RawFilters        map[string]interface{} `json:"raw_filters" jsonschema_description:"Raw filter dictionary for TheHive queries. Format: {operator: {_field: <field>, _value: <value>}}. Operators: _and, _or, _not, _eq, _ne, _gt, _gte, _lt, _lte, _between (_from, _to), _like, _in, _startsWith, _endsWith, _has, _id, _any, _match."`
	SortBy            string                 `json:"sort_by" jsonschema_description:"Column to sort the results by."`
	SortOrder         string                 `json:"sort_order" jsonschema_description:"Sort order ('asc' for ascending, 'desc' for descending)."`
	NumResults        int                    `json:"num_results" jsonschema_description:"Number of results to return. Default is 10."`
	KeptColumns       []string               `json:"kept_columns" jsonschema_description:"List of columns to keep in the output. Default is ['_id', 'title', 'url']"`
	ExtraData         []string               `json:"extra_data" jsonschema_description:"List of additional data fields to include in the output."`
	AdditionalQueries []string               `json:"additional_queries" jsonschema_description:"List of additional queries to perform on the results to enrich them with related data."`
}

func (t *SearchTool) parseQuery(ctx context.Context, params *searchParams, additionalMessages []mcp.PromptMessage) (*FilterResult, error) {
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

func (t *SearchTool) buildHiveQuery(params *searchParams, filters *FilterResult) (thehive.InputQuery, error) {
	// Build operations
	listOp := t.buildListOperation(params.EntityType)
	filterOp := t.buildFilterOperation(filters.RawFilters)
	sortOp := t.buildSortOperation(filters.SortBy, filters.SortOrder)
	pageOp := t.buildPagingOperation(filters.NumResults, filters.ExtraData)

	// Exclude unneeded fields
	excludedFields := t.getExcludedFields(params.EntityType, filters.KeptColumns, filters.ExtraData)

	hiveQuery := thehive.InputQuery{
		Query: []thehive.InputQueryNamedOperation{
			thehive.InputQueryGenericOperationAsInputQueryNamedOperation(listOp),
			thehive.MapmapOfStringAnyAsInputQueryNamedOperation(filterOp),
			thehive.InputQuerySortOperationAsInputQueryNamedOperation(sortOp),
			thehive.InputQueryPagingOperationAsInputQueryNamedOperation(pageOp),
		},
		ExcludeFields: excludedFields,
	}

	// Debug logging
	if queryJSON, err := json.MarshalIndent(hiveQuery, "", "  "); err == nil {
		slog.Info("Built TheHive query", "json", string(queryJSON))
	}

	return hiveQuery, nil
}

func (t *SearchTool) buildListOperation(entityType string) *thehive.InputQueryGenericOperation {
	operationName := fmt.Sprintf("list%s", capitalize(entityType))
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

	// First try to cast to []interface{}
	resultsInterface, ok := results.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result type from TheHive API. Expected []interface{} but got %T: %v", results, results)
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

// Result processing
func (t *SearchTool) formatResults(results []map[string]interface{}, params *searchParams, filters map[string]interface{}) (*mcp.CallToolResult, error) {
	// Convert to map slice
	resultsMap, err := t.convertToMapSlice(results)
	if err != nil {
		return nil, fmt.Errorf("failed to convert results: %w", err)
	}

	// Serialize entities (format dates, etc.)
	parsedResults, err := t.parseDateFields(resultsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse entities: %w", err)
	}

	// Build response
	response := map[string]interface{}{
		"results":    parsedResults,
		"count":      len(parsedResults),
		"entityType": params.EntityType,
		"query":      params.Query,
		"filters":    filters,
	}

	return utils.NewToolResultJSONUnescaped(response), nil
}

func (t *SearchTool) convertToMapSlice(results []map[string]interface{}) ([]map[string]interface{}, error) {
	resultBytes, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal results: %w", err)
	}

	var resultsMap []map[string]interface{}
	if err := json.Unmarshal(resultBytes, &resultsMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal results: %w", err)
	}

	return resultsMap, nil
}

func (t *SearchTool) parseDateFields(entities []map[string]interface{}) ([]map[string]interface{}, error) {
	parsedResults := make([]map[string]interface{}, 0, len(entities))

	for _, entity := range entities {
		parsedEntity, err := utils.ParseDateFields(entity)
		if err != nil {
			return nil, fmt.Errorf("failed to parse entity: %w", err)
		}
		parsedResults = append(parsedResults, parsedEntity)
	}

	return parsedResults, nil
}

// Helper methods
func (t *SearchTool) getExcludedFields(entityType string, keptColumns []string, extraData []string) []string {
	var baseModel any
	switch entityType {
	case "alert":
		baseModel = thehive.OutputAlert{}
	case "case":
		baseModel = thehive.OutputCase{}
	case "task":
		baseModel = thehive.OutputTask{}
	case "observable":
		baseModel = thehive.OutputObservable{}
	default:
		return []string{}
	}

	allFields := utils.GetJSONFields(baseModel)
	excludeFields := make([]string, 0)

	for _, field := range allFields {
		if !contains(keptColumns, field) {
			if field == "extraData" && len(extraData) > 0 {
				continue
			}
			excludeFields = append(excludeFields, field)
		}
	}
	return excludeFields
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
