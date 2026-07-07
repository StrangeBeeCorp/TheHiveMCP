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
	categoryDescriptions map[string]string
}

type resourceEntry struct {
	resource mcp.Resource
	handler  func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error)
}

// NewResourceRegistry returns an empty ResourceRegistry ready for registration.
func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{
		resources:            make(map[string]resourceEntry),
		categoryDescriptions: make(map[string]string),
	}
}

// RegisterCategoryMetadata stores category and subcategory descriptions from catalog
func (r *ResourceRegistry) RegisterCategoryMetadata(categories []map[string]any) {
	for _, cat := range categories {
		name, desc, ok := nameAndDescription(cat)
		if !ok {
			continue
		}

		r.categoryDescriptions[name] = desc

		subcats, ok := cat["subcategories"].([]map[string]any)
		if !ok {
			continue
		}

		for _, subcat := range subcats {
			subName, subDesc, ok := nameAndDescription(subcat)
			if !ok {
				continue
			}

			r.categoryDescriptions[fmt.Sprintf("%s/%s", name, subName)] = subDesc
		}
	}
}

// nameAndDescription extracts the name and description string values from a
// catalog map; ok is false when either key is missing or not a string.
func nameAndDescription(m map[string]any) (name, desc string, ok bool) {
	name, ok = m[keyName].(string)
	if !ok {
		return "", "", false
	}

	desc, ok = m[keyDescription].(string)
	if !ok {
		return "", "", false
	}

	return name, desc, true
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
func (r *ResourceRegistry) ListByCategory(category string) ([]map[string]any, []map[string]any) {
	var resources []map[string]any

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
			resources = append(resources, map[string]any{
				"uri":          uri,
				keyName:        entry.resource.Name,
				keyDescription: entry.resource.Description,
			})
		} else {
			parts := strings.Split(relativePath, "/")
			subcategory := parts[0]
			subcategoriesMap[subcategory] = true
		}
	}

	var subcategories []map[string]any

	for subcat := range subcategoriesMap {
		fullPath := category
		if fullPath != "" {
			fullPath += "/"
		}

		fullPath += subcat

		subcategories = append(subcategories, map[string]any{
			keyName:        subcat,
			"uri":          fmt.Sprintf("hive://%s/", fullPath),
			keyDescription: r.categoryDescriptions[fullPath],
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
