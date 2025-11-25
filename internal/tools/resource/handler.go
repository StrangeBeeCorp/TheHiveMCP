package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ResourceTool) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uri := req.GetString("uri", "")
	category := req.GetString("category", "")

	// Validate mutual exclusivity
	if uri != "" && category != "" {
		return mcp.NewToolResultError("uri and category parameters are mutually exclusive. Provide either 'uri' for a specific resource or 'category' to browse, but not both."), nil
	}

	// Route to appropriate handler
	if uri != "" {
		return t.fetchResource(ctx, uri)
	} else if category != "" {
		return t.browseCategory(ctx, category)
	} else {
		return t.listCategories(ctx)
	}
}

// List all categories (catalog)
func (t *ResourceTool) listCategories(ctx context.Context) (*mcp.CallToolResult, error) {
	slog.Info("Listing all resource categories")

	// Fetch the catalog resource
	return t.fetchResource(ctx, "hive://catalog")
}

// Browse resources in a category
func (t *ResourceTool) browseCategory(ctx context.Context, category string) (*mcp.CallToolResult, error) {
	slog.Info("Browsing category", "category", category)

	// Normalize category name
	category = strings.TrimPrefix(category, "hive://")
	category = strings.TrimSuffix(category, "/")

	// Get resources and subcategories in this category
	resources, subcategories := t.resourceRegistry.ListByCategory(category)

	if len(resources) == 0 && len(subcategories) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("category '%s' not found or empty. Use get-resource without parameters to see available categories, or try: 'schema', 'metadata', 'docs', 'config'.", category)), nil
	}

	response := map[string]interface{}{
		"category":      category,
		"uri":           fmt.Sprintf("hive://%s/", category),
		"subcategories": subcategories,
		"resources":     resources,
	}

	return utils.NewToolResultJSONUnescaped(response)
}

// Fetch a specific resource
func (t *ResourceTool) fetchResource(ctx context.Context, uri string) (*mcp.CallToolResult, error) {
	slog.Info("Fetching resource", "uri", uri)

	if !strings.HasPrefix(uri, "hive://") {
		uri = "hive://" + uri
	}

	// Assuming resourceRegistry.Get exists
	resource, handler, err := t.resourceRegistry.Get(uri)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("resource not found: %s. Use get-resource without parameters to browse available resources, or check that the URI is correct (e.g., 'hive://schema/alert').", uri)), nil
	}

	// FIX 1: URI belongs in the Params struct
	readRequest := mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: uri,
		},
	}

	// Handler returns []mcp.ResourceContents
	contents, err := handler(ctx, readRequest)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to fetch resource: %v. This may be due to network issues, authentication problems, or the resource being temporarily unavailable.", err)), nil
	}

	if len(contents) == 0 {
		return mcp.NewToolResultError("resource returned no content. The resource exists but contains no data. This may be a temporary issue or the resource may be empty."), nil
	}

	// FIX 2: Assert the type of the first element (contents)
	// Use mcp.TextResourceContents to access Text and MIMEType
	textContent, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("resource content is not readable text or JSON compatible: %T. The resource may be a binary file or in an unsupported format.", contents)), nil
	}

	contentText := textContent.Text
	mimeType := textContent.MIMEType

	// Parse the content
	var data interface{}
	if err := json.Unmarshal([]byte(contentText), &data); err != nil {
		// If not JSON, return as text
		response := map[string]interface{}{
			"uri":      uri,
			"name":     resource.Name,
			"mimeType": mimeType,
			"content":  contentText,
		}
		return utils.NewToolResultJSONUnescaped(response)
	}

	// Return structured response
	response := map[string]interface{}{
		"uri":      uri,
		"name":     resource.Name,
		"mimeType": mimeType,
		"data":     data,
	}

	return utils.NewToolResultJSONUnescaped(response)
}
