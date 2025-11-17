package resource

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/resources"
	"github.com/mark3labs/mcp-go/mcp"
)

type ResourceTool struct {
	resourceRegistry *resources.ResourceRegistry
}

func NewResourceTool(registry *resources.ResourceRegistry) *ResourceTool {
	return &ResourceTool{
		resourceRegistry: registry,
	}
}

func (t *ResourceTool) Definition() mcp.Tool {
	return mcp.NewTool(
		"get-resource",
		mcp.WithDescription(`Access TheHive resources for documentation, schemas, and metadata.

Resources are organized hierarchically:
- hive://catalog - Directory of all categories
- hive://config/* - Session and system info
- hive://schema/* - Entity field definitions
- hive://metadata/* - Available options and choices
- hive://docs/* - Documentation and guides

Usage patterns:
1. Discover: Call without parameters to list all categories
2. Browse: Provide category to list resources (e.g., "schema")
3. Fetch: Provide full URI to get specific resource (e.g., "hive://schema/alert")

Examples:
- List all categories: get-resource()
- List schemas: get-resource(category="schema")
- Get alert schema: get-resource(uri="hive://schema/alert")
- Get case docs: get-resource(uri="hive://docs/entities/case")

The get-resource tool is the entry point for exploring TheHive's capabilities. Start by browsing the catalog, then drill down into categories and specific resources as needed. This allows you to understand available entities, their fields, and how to interact with them effectively.
You can then make informed calls to other tools like search, create, or update using the information obtained here. Always refer to the latest server resources to ensure accuracy and compatibility.
`),
		mcp.WithString(
			"uri",
			mcp.Description("Full resource URI (e.g., 'hive://schema/alert'). Mutually exclusive with category."),
		),
		mcp.WithString(
			"category",
			mcp.Description("Category to browse (e.g., 'schema', 'metadata', 'docs'). Mutually exclusive with uri."),
		),
	)
}
