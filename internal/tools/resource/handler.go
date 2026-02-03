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

	// If no URI provided, list catalog
	if uri == "" {
		slog.Info("Listing all resource categories")
		return t.fetchUnified(ctx, "hive://catalog")
	}

	// Normalize URI: add prefix if needed, remove trailing slash
	uri = normalizeURI(uri)
	slog.Info("Fetching resource", "uri", uri)

	return t.fetchUnified(ctx, uri)
}

// normalizeURI ensures consistent URI format
func normalizeURI(uri string) string {
	// Add hive:// prefix if missing
	if !strings.HasPrefix(uri, "hive://") {
		uri = "hive://" + uri
	}

	// Remove trailing slash for consistency
	uri = strings.TrimSuffix(uri, "/")

	return uri
}

// fetchUnified attempts to fetch a resource and/or browse a category at the given URI
// Returns a unified response with the resource content (if it exists), subcategories, and resources
func (t *ResourceTool) fetchUnified(ctx context.Context, uri string) (*mcp.CallToolResult, error) {
	// Extract category path from URI
	category := strings.TrimPrefix(uri, "hive://")

	// Extract parameters from uri
	var parameters map[string]any
	var err error
	if strings.Contains(uri, "?") {
		uri, parameters, err = utils.ParseURIParameters(uri)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse URI parameters: %v", err)), nil
		}
	}

	// Try to fetch as a specific resource first
	resource, handler, resourceErr := t.resourceRegistry.Get(uri)

	var resourceData map[string]interface{}
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
			return mcp.NewToolResultError(fmt.Sprintf("failed to fetch resource: %v. This may be due to network issues, authentication problems, or the resource being temporarily unavailable.", err)), nil
		}

		if len(contents) == 0 {
			return mcp.NewToolResultError("resource returned no content. The resource exists but contains no data. This may be a temporary issue or the resource may be empty."), nil
		}

		// Assert the type of the first element (contents)
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
			resourceData = map[string]interface{}{
				"uri":      uri,
				"name":     resource.Name,
				"mimeType": mimeType,
				"content":  contentText,
			}
		} else {
			// Return structured JSON data
			resourceData = map[string]interface{}{
				"uri":      uri,
				"name":     resource.Name,
				"mimeType": mimeType,
				"data":     data,
			}
		}
	}

	// If we found a specific resource, return it (no need for subcategories/resources)
	if resourceData != nil {
		return utils.NewToolResultJSONUnescaped(resourceData), nil
	}

	// Also check for subcategories and resources at this path
	resources, subcategories := t.resourceRegistry.ListByCategory(category)

	// If we didn't find a resource, check if we found a category with contents
	if len(resources) > 0 || len(subcategories) > 0 {
		response := map[string]interface{}{
			"uri":           uri,
			"subcategories": subcategories,
			"resources":     resources,
		}
		return utils.NewToolResultJSONUnescaped(response), nil
	}

	// Nothing found
	return mcp.NewToolResultError(fmt.Sprintf("resource not found: %s. Use get-resource without parameters to browse available resources.", uri)), nil
}
