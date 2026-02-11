package resource

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/tools"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/mark3labs/mcp-go/mcp"
)

func (t *ResourceTool) Handle(ctx context.Context, req mcp.CallToolRequest, params GetResourceParams) (GetResourceResult, error) {
	slog.Info("Fetching resource", "uri", params.URI)

	return t.fetchUnified(ctx, params.URI)
}

// fetchUnified attempts to fetch a resource and/or browse a category at the given URI
// Returns a unified response with the resource content (if it exists), subcategories, and resources
func (t *ResourceTool) fetchUnified(ctx context.Context, uri string) (GetResourceResult, error) {
	// Extract category path from URI
	category := strings.TrimPrefix(uri, "hive://")

	// Extract parameters from uri
	var parameters map[string]any
	var err error
	if strings.Contains(uri, "?") {
		uri, parameters, err = utils.ParseURIParameters(uri)
		if err != nil {
			return GetResourceResult{}, tools.NewToolError("failed to parse URI parameters").Cause(err)
		}
	}

	// Try to fetch as a specific resource first
	resource, handler, resourceErr := t.resourceRegistry.Get(uri)

	if resourceErr == nil {
		// It's a resource, fetch its content
		readRequest := mcp.ReadResourceRequest{
			Params: mcp.ReadResourceParams{
				URI:       uri,
				Arguments: parameters,
			},
		}

		contents, err := handler(ctx, readRequest)
		if err != nil {
			return GetResourceResult{}, tools.NewToolError("failed to fetch resource").Cause(err).
				Hint("This may be due to network issues, authentication problems, or the resource being temporarily unavailable")
		}

		if len(contents) == 0 {
			return GetResourceResult{}, tools.NewToolError("resource returned no content").
				Hint("The resource exists but contains no data").
				Hint("This may be a temporary issue or the resource may be empty")
		}

		// Assert the type of the first element (contents)
		textContent, ok := contents[0].(mcp.TextResourceContents)
		if !ok {
			return GetResourceResult{}, tools.NewToolErrorf("resource content is not readable text or JSON compatible: %T", contents).
				Hint("The resource may be a binary file or in an unsupported format")
		}

		contentText := textContent.Text
		mimeType := textContent.MIMEType

		// Parse the content
		var data interface{}
		resourceContent := NewResourceContent(uri, resource.Name, mimeType)

		if err := json.Unmarshal([]byte(contentText), &data); err != nil {
			// If not JSON, return as text
			resourceContent.SetTextContent(contentText)
		} else {
			// Return structured JSON data
			resourceContent.SetDataContent(data)
		}

		return *NewResourceResult(resourceContent), nil
	}

	// Check for subcategories and resources at this path
	resources, subcategories := t.resourceRegistry.ListByCategory(category)

	// If we found a category with contents, return it
	if len(resources) > 0 || len(subcategories) > 0 {
		categoryBrowse := NewCategoryBrowse(uri, subcategories, resources)
		return *NewCategoryResult(categoryBrowse), nil
	}

	// Nothing found
	return GetResourceResult{}, tools.NewToolErrorf("resource not found: %s", uri).
		Hint("Use get-resource without parameters to browse available resources")
}
