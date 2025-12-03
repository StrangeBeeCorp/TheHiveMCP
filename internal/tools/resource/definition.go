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

Usage:
- Call without parameters to list all categories
- Provide a URI to fetch resources, subcategories, and content at that path

The tool returns a unified response that includes:
- The resource content (if the URI points to a specific resource)
- Subcategories under the path (if any exist)
- Resources available at the path (if any exist)

Examples:
- List all categories: get-resource()
- Browse schemas: get-resource(uri="hive://schema")
- Browse automation metadata: get-resource(uri="hive://metadata/automation")
- Get alert schema: get-resource(uri="hive://schema/alert")
- Get case docs: get-resource(uri="hive://docs/entities/case")

The get-resource tool is the entry point for exploring TheHive's capabilities. Start by browsing the catalog, then drill down into specific resources as needed. This allows you to understand available entities, their fields, and how to interact with them effectively.
You can then make informed calls to other tools like search, create, or update using the information obtained here. Always refer to the latest server resources to ensure accuracy and compatibility.
`),
		mcp.WithString(
			"uri",
			mcp.Description("Resource URI to query (e.g., 'hive://schema/alert', 'hive://metadata/automation'). Omit to list all categories."),
		),
	)
}
