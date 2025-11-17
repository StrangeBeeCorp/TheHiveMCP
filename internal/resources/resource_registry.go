package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ResourceRegistry manages resource registration and lookup
type ResourceRegistry struct {
	resources            map[string]resourceEntry
	categoryDescriptions map[string]string // Store descriptions from catalog
}

type resourceEntry struct {
	resource mcp.Resource
	handler  func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error)
}

func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{
		resources:            make(map[string]resourceEntry),
		categoryDescriptions: make(map[string]string),
	}
}

// RegisterCategoryMetadata stores category and subcategory descriptions from catalog
func (r *ResourceRegistry) RegisterCategoryMetadata(categories []map[string]interface{}) {
	for _, cat := range categories {
		name := cat["name"].(string)
		desc := cat["description"].(string)
		r.categoryDescriptions[name] = desc

		// Register subcategories if they exist
		if subcats, ok := cat["subcategories"].([]map[string]interface{}); ok {
			for _, subcat := range subcats {
				subName := subcat["name"].(string)
				subDesc := subcat["description"].(string)
				fullPath := fmt.Sprintf("%s/%s", name, subName)
				r.categoryDescriptions[fullPath] = subDesc
			}
		}
	}
}

// Register adds a resource to the registry
func (r *ResourceRegistry) Register(resource mcp.Resource, handler func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error)) {
	r.resources[resource.URI] = resourceEntry{
		resource: resource,
		handler:  handler,
	}
}

// Get retrieves a resource and its handler
func (r *ResourceRegistry) Get(uri string) (mcp.Resource, func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error), error) {
	entry, exists := r.resources[uri]
	if !exists {
		return mcp.Resource{}, nil, fmt.Errorf("resource not found: %s. Use get-resource without parameters to see available resources, or check the URI format (e.g., 'hive://schema/alert')", uri)
	}
	return entry.resource, entry.handler, nil
}

// ListByCategory returns resources and subcategories at the specified level
func (r *ResourceRegistry) ListByCategory(category string) ([]map[string]interface{}, []map[string]interface{}) {
	var resources []map[string]interface{}
	subcategoriesMap := make(map[string]bool)

	prefix := fmt.Sprintf("hive://%s/", category)
	if category == "" {
		prefix = "hive://"
	}

	for uri, entry := range r.resources {
		// Skip catalog
		if uri == "hive://catalog" {
			continue
		}

		// Check if this URI is under our category
		if !strings.HasPrefix(uri, prefix) {
			continue
		}

		// Get the relative path after the prefix
		relativePath := strings.TrimPrefix(uri, prefix)

		// Count slashes in relative path to determine depth
		slashCount := strings.Count(relativePath, "/")

		if slashCount == 0 {
			// Direct child resource (no more slashes)
			resources = append(resources, map[string]interface{}{
				"uri":         uri,
				"name":        entry.resource.Name,
				"description": entry.resource.Description,
			})
		} else {
			// Has more path segments, so there's a subcategory
			// Extract the immediate subcategory name
			parts := strings.Split(relativePath, "/")
			subcategory := parts[0]
			subcategoriesMap[subcategory] = true
		}
	}

	// Convert subcategories map to slice with descriptions from catalog
	var subcategories []map[string]interface{}
	for subcat := range subcategoriesMap {
		fullPath := category
		if fullPath != "" {
			fullPath += "/"
		}
		fullPath += subcat

		subcategories = append(subcategories, map[string]interface{}{
			"name":        subcat,
			"uri":         fmt.Sprintf("hive://%s/", fullPath),
			"description": r.categoryDescriptions[fullPath],
		})
	}

	return resources, subcategories
}

// RegisterAll registers all resources with the MCP server
func (r *ResourceRegistry) RegisterAll(s *server.MCPServer) {
	for _, entry := range r.resources {
		s.AddResource(entry.resource, entry.handler)
	}
}
