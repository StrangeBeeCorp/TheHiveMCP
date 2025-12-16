package search

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *SearchTool) Definition() mcp.Tool {
	return mcp.NewTool(
		"search-entities",
		mcp.WithDescription(`Search for entities in TheHive using natural language queries.

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

Only use this tool with precise queries related to searching TheHive entities. It is highly recommended to refer to the [entity]-schema from server resources for available fields and types. Every investigation should start by exploring the available entities and their fields using the get-resource tool.`),
		mcp.WithString(
			"entity-type",
			mcp.Required(),
			mcp.Enum(string(types.EntityTypeAlert), string(types.EntityTypeCase), string(types.EntityTypeTask), string(types.EntityTypeObservable)),
			mcp.Description("Type of entity to search for."),
		),
		mcp.WithString(
			"query",
			mcp.Required(),
			mcp.Description("Natural language query describing what entities you want to find. This query will be converted to TheHive filters using a specialized AI Agent. The filters will be returned along with the search results for transparency. If the results are not as expected, consider documenting yourself about the filters in the resources, that will help you refine your query."),
		),
		mcp.WithString(
			"sort-by",
			mcp.Description("Column to sort the results by. Leave empty to let the query determine sorting."),
			mcp.DefaultString("_createdAt"),
		),
		mcp.WithString(
			"sort-order",
			mcp.Description("Sort order ('asc' or 'desc'). Default is 'desc'."),
			mcp.Enum("asc", "desc"),
			mcp.DefaultString("desc"),
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Number of results to return. Default is 10."),
			mcp.DefaultNumber(10),
		),
		mcp.WithArray(
			"extra-columns",
			mcp.Description("List of columns to keep in the output. Default is ['_id', 'title']. Query the [entity]-schema from server resources for available columns."),
			mcp.DefaultArray([]string{"_id", "title"}),
		),
		mcp.WithArray(
			"extra-data",
			mcp.Description("List of additional data fields to include in the output. Query the [entity]-schema from server resources for available extra data fields."),
			mcp.DefaultArray([]string{}),
		),
		mcp.WithArray(
			"additional-queries",
			mcp.Description("Additional queries to perform on the results. Differnt queries are supported depending on the entity type. For example, for cases you can fetch tasks or observables related to the found cases. Use this to enrich the results with related data. Refer to the entity schema from server resources for supported additional queries."),
			mcp.DefaultArray([]string{}),
		),
	)
}

type SearchTool struct{}

func NewSearchTool() *SearchTool {
	return &SearchTool{}
}
