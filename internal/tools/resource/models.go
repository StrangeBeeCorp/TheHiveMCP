package resource

import "github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"

// GetResourceToolDescription is the human-readable description advertised for the get-resource tool.
const GetResourceToolDescription = `Access TheHive resources for documentation, schemas, and metadata.

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
You can then make informed calls to other tools like search, create, or update using the information obtained here. Always refer to the latest server resources to ensure accuracy and compatibility.`

// GetResourceParams defines the input parameters for the get-resource tool
type GetResourceParams struct {
	URI string `json:"uri,omitempty" jsonschema_description:"Resource URI to query (e.g., 'hive://schema/alert', 'hive://metadata/automation'). Omit to list all categories."`
}

// Content represents a specific resource with its content.
type Content struct {
	URI      string `json:"uri"`
	Name     string `json:"name"`
	MIMEType string `json:"mimeType,omitempty"`
	Content  string `json:"content,omitempty"`
	Data     any    `json:"data,omitempty"`
}

// NewResourceContent creates a Content for the resource identified by uri.
func NewResourceContent(uri, name, mimeType string) *Content {
	return &Content{
		URI:      uri,
		Name:     name,
		MIMEType: mimeType,
	}
}

// SetTextContent sets the plain-text body of the resource and returns rc for chaining.
func (rc *Content) SetTextContent(content string) *Content {
	rc.Content = content
	return rc
}

// SetDataContent sets the structured data body of the resource and returns rc for chaining.
func (rc *Content) SetDataContent(data any) *Content {
	rc.Data = data
	return rc
}

// CategoryBrowse represents a directory listing of resources and subcategories
type CategoryBrowse struct {
	URI           string           `json:"uri"`
	Subcategories []map[string]any `json:"subcategories,omitempty"`
	Resources     []map[string]any `json:"resources,omitempty"`
}

// NewCategoryBrowse creates a CategoryBrowse listing for the given uri.
func NewCategoryBrowse(uri string, subcategories []map[string]any, resources []map[string]any) *CategoryBrowse {
	return &CategoryBrowse{
		URI:           uri,
		Subcategories: subcategories,
		Resources:     resources,
	}
}

// GetResourceResult is the unified response type for get-resource operations
// Either Resource OR Category will be populated, never both
type GetResourceResult struct {
	Resource *Content        `json:"resource,omitempty"`
	Category *CategoryBrowse `json:"category,omitempty"`
}

// NewResourceResult wraps resource content in a GetResourceResult.
func NewResourceResult(resource *Content) *GetResourceResult {
	return &GetResourceResult{
		Resource: resource,
	}
}

// NewCategoryResult wraps a category listing in a GetResourceResult.
func NewCategoryResult(category *CategoryBrowse) *GetResourceResult {
	return &GetResourceResult{
		Category: category,
	}
}

// Unwrap implements utils.Unwrapper to flatten the union for serialization.
func (r GetResourceResult) Unwrap() any { return utils.UnwrapUnion(r) }
