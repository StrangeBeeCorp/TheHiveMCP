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

func (t *ResourceTool) fetchUnified(ctx context.Context, uri string) (GetResourceResult, error) {
	category := strings.TrimPrefix(uri, "hive://")

	var parameters map[string]any
	var err error
	if strings.Contains(uri, "?") {
		uri, parameters, err = utils.ParseURIParameters(uri)
		if err != nil {
			return GetResourceResult{}, tools.NewToolError("failed to parse URI parameters").Cause(err)
		}
	}

	// Try to fetch as a specific resource before falling back to category browse
	resource, handler, resourceErr := t.resourceRegistry.Get(uri)

	if resourceErr == nil {
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

		textContent, ok := contents[0].(mcp.TextResourceContents)
		if !ok {
			return GetResourceResult{}, tools.NewToolErrorf("resource content is not readable text or JSON compatible: %T", contents).
				Hint("The resource may be a binary file or in an unsupported format")
		}

		contentText := textContent.Text
		mimeType := textContent.MIMEType

		var data interface{}
		resourceContent := NewResourceContent(uri, resource.Name, mimeType)

		if err := json.Unmarshal([]byte(contentText), &data); err != nil {
			resourceContent.SetTextContent(contentText)
		} else {
			resourceContent.SetDataContent(data)
		}

		return *NewResourceResult(resourceContent), nil
	}

	resources, subcategories := t.resourceRegistry.ListByCategory(category)

	if len(resources) > 0 || len(subcategories) > 0 {
		categoryBrowse := NewCategoryBrowse(uri, subcategories, resources)
		return *NewCategoryResult(categoryBrowse), nil
	}

	return GetResourceResult{}, tools.NewToolErrorf("resource not found: %s", uri).
		Hint("Use get-resource without parameters to browse available resources")
}
