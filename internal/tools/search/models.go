package search

import "github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"

const SearchEntitiesToolDescription = `Search for entities in TheHive using natural language queries.

The query will be translated to TheHive filters using AI. You can use natural language to describe what you're looking for.

Examples:
- "high severity alerts from last week"
- "open cases assigned to john@example.com"
- "all tasks with status waiting"
- "observables containing malware in the last month"
- "latest phishing alerts with severity greater than 2"

The search understands:
- Severity levels (high, medium, low, critical)
- Status and stage filters
- Date ranges (last week, last month, yesterday, etc.)
- Assignees and ownership
- Tags and keywords
- Sorting (latest, oldest, newest)

When asked for statistics, it is recommended to use count=true to get only the count of matching entities. Otherwise, the tool will be limited by the limit parameter.
Only use this tool with precise queries related to searching TheHive entities. It is highly recommended to refer to the [entity]-schema from server resources for available fields and types. Every investigation should start by exploring the available entities and their fields using the get-resource tool.`

type SearchEntitiesParams struct {
	EntityType        string   `json:"entity-type" jsonschema:"enum=alert,enum=case,enum=task,enum=observable,required=true" jsonschema_description:"Type of entity to search for."`
	Query             string   `json:"query" jsonschema:"required=true" jsonschema_description:"Natural language query describing what entities you want to find. This query will be converted to TheHive filters using a specialized AI Agent. The filters will be returned along with the search results for transparency. If the results are not as expected, consider documenting yourself about the filters in the resources, that will help you refine your query."`
	SortBy            string   `json:"sort-by,omitempty" jsonschema:"default=_createdAt" jsonschema_description:"Column to sort the results by. Leave empty to let the query determine sorting."`
	SortOrder         string   `json:"sort-order,omitempty" jsonschema:"enum=asc,enum=desc,default=desc" jsonschema_description:"Sort order ('asc' or 'desc'). Default is 'desc'."`
	Limit             int      `json:"limit,omitempty" jsonschema:"default=10" jsonschema_description:"Number of results to return. Default is 10. Not applicable if count=true."`
	ExtraColumns      []string `json:"extra-columns,omitempty" jsonschema_description:"List of columns to keep in the output. Defaults are entity-specific: alerts include severity/status, cases include status/severity, tasks include assignee, etc. Query the [entity]-schema from server resources for available columns."`
	ExtraData         []string `json:"extra-data,omitempty" jsonschema_description:"List of additional data fields to include in the output. Query the [entity]-schema from server resources for available extra data fields."`
	AdditionalQueries []string `json:"additional-queries,omitempty" jsonschema_description:"Additional queries to perform on the results. Different queries are supported depending on the entity type. For example, for cases you can fetch tasks or observables related to the found cases. Use this to enrich the results with related data. Refer to the entity schema from server resources for supported additional queries."`
	Count             bool     `json:"count,omitempty" jsonschema_description:"If true, returns only the count of matching entities instead of the entities themselves."`
}

type SearchEntitiesResult struct {
	Count      int                      `json:"count"`
	CountOnly  bool                     `json:"countOnly"`
	EntityType string                   `json:"entityType"`
	Results    []map[string]interface{} `json:"results,omitempty"`
	RawFilters map[string]interface{}   `json:"rawFilters"`
}

func NewSearchEntitiesResult(results []map[string]interface{}, params SearchEntitiesParams, filters map[string]interface{}) (SearchEntitiesResult, error) {
	var countValue int
	if params.Count {
		if len(results) == 0 {
			return SearchEntitiesResult{}, tools.NewToolError("no results returned for count query").Hint("Ensure the query returns at least one result with a count field when count=true").Schema(params.EntityType, "")
		}
		floatCountValue, ok := results[0]["_count"].(float64)
		if !ok {
			return SearchEntitiesResult{}, tools.NewToolError("failed to parse count from results").Hint("Ensure the query returns a count field when count=true").Schema(params.EntityType, "")
		}
		countValue = int(floatCountValue)
	} else {
		countValue = len(results)
	}
	return SearchEntitiesResult{
		Count:      countValue,
		CountOnly:  params.Count,
		EntityType: params.EntityType,
		Results:    results,
		RawFilters: filters,
	}, nil
}

// Query parsing - internal helper struct
type FilterResult struct {
	RawFilters        map[string]interface{} `json:"raw_filters" jsonschema_description:"Raw filter dictionary for TheHive queries. Format: {operator: {_field: <field>, _value: <value>}}. Operators: _and, _or, _not, _eq, _ne, _gt, _gte, _lt, _lte, _between (_from, _to), _like, _in, _startsWith, _endsWith, _has, _id, _any, _match."`
	SortBy            string                 `json:"sort_by" jsonschema_description:"Column to sort the results by."`
	SortOrder         string                 `json:"sort_order" jsonschema_description:"Sort order ('asc' for ascending, 'desc' for descending)."`
	NumResults        int                    `json:"num_results" jsonschema_description:"Number of results to return. Default is 10."`
	KeptColumns       []string               `json:"kept_columns" jsonschema_description:"List of columns to keep in the output. Default is ['_id', 'title', 'url']"`
	ExtraData         []string               `json:"extra_data" jsonschema_description:"List of additional data fields to include in the output."`
	AdditionalQueries []string               `json:"additional_queries" jsonschema_description:"List of additional queries to perform on the results to enrich them with related data."`
}
