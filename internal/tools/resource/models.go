package resource

import "github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"

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

// ResourceContent represents a specific resource with content
type ResourceContent struct {
	URI      string      `json:"uri"`
	Name     string      `json:"name"`
	MIMEType string      `json:"mimeType,omitempty"`
	Content  string      `json:"content,omitempty"`
	Data     interface{} `json:"data,omitempty"`
}

func NewResourceContent(uri, name, mimeType string) *ResourceContent {
	return &ResourceContent{
		URI:      uri,
		Name:     name,
		MIMEType: mimeType,
	}
}

func (rc *ResourceContent) SetTextContent(content string) *ResourceContent {
	rc.Content = content
	return rc
}

func (rc *ResourceContent) SetDataContent(data interface{}) *ResourceContent {
	rc.Data = data
	return rc
}

// CategoryBrowse represents a directory listing of resources and subcategories
type CategoryBrowse struct {
	URI           string                   `json:"uri"`
	Subcategories []map[string]interface{} `json:"subcategories,omitempty"`
	Resources     []map[string]interface{} `json:"resources,omitempty"`
}

func NewCategoryBrowse(uri string, subcategories []map[string]interface{}, resources []map[string]interface{}) *CategoryBrowse {
	return &CategoryBrowse{
		URI:           uri,
		Subcategories: subcategories,
		Resources:     resources,
	}
}

// GetResourceResult is the unified response type for get-resource operations
// Either Resource OR Category will be populated, never both
type GetResourceResult struct {
	Resource *ResourceContent `json:"resource,omitempty"`
	Category *CategoryBrowse  `json:"category,omitempty"`
}

func NewResourceResult(resource *ResourceContent) *GetResourceResult {
	return &GetResourceResult{
		Resource: resource,
	}
}

func NewCategoryResult(category *CategoryBrowse) *GetResourceResult {
	return &GetResourceResult{
		Category: category,
	}
}

// Unwrap implements utils.Unwrapper to flatten the union for serialization.
func (r GetResourceResult) Unwrap() any { return utils.UnwrapUnion(r) }
