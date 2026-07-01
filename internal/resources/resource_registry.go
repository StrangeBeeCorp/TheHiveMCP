package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type ResourceRegistry struct {
	resources            map[string]resourceEntry
	categoryDescriptions map[string]string
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

func (r *ResourceRegistry) RegisterCategoryMetadata(categories []map[string]interface{}) {
	for _, cat := range categories {
		name := cat["name"].(string)
		desc := cat["description"].(string)
		r.categoryDescriptions[name] = desc

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

func (r *ResourceRegistry) Register(resource mcp.Resource, handler func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error)) {
	r.resources[resource.URI] = resourceEntry{
		resource: resource,
		handler:  handler,
	}
}

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
		if uri == "hive://catalog" {
			continue
		}

		if !strings.HasPrefix(uri, prefix) {
			continue
		}

		relativePath := strings.TrimPrefix(uri, prefix)
		slashCount := strings.Count(relativePath, "/")

		if slashCount == 0 {
			resources = append(resources, map[string]interface{}{
				"uri":         uri,
				"name":        entry.resource.Name,
				"description": entry.resource.Description,
			})
		} else {
			parts := strings.Split(relativePath, "/")
			subcategory := parts[0]
			subcategoriesMap[subcategory] = true
		}
	}

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

func (r *ResourceRegistry) RegisterAll(s *server.MCPServer) {
	for _, entry := range r.resources {
		s.AddResource(entry.resource, entry.handler)
	}
}
