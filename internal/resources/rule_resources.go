package resources

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// GetFormattingRuleHandler returns TheHive formatting rule content.
func GetFormattingRuleHandler() ([]mcp.ResourceContents, error) {
	return getRuleContent("formatting")
}

// GetIntegrityRuleHandler returns TheHive integrity rule content.
func GetIntegrityRuleHandler() ([]mcp.ResourceContents, error) {
	return getRuleContent("integrity")
}

// GetFilteringRuleHandler returns TheHive filtering rule content.
func GetFilteringRuleHandler() ([]mcp.ResourceContents, error) {
	return getRuleContent("filtering")
}

// RegisterRuleResources registers the rule resources on the registry.
func RegisterRuleResources(registry *ResourceRegistry) {
	formattingRule := mcp.NewResource(
		"hive://rule/formatting",
		"Formatting Rule",
		mcp.WithResourceDescription("TheHive formatting rule"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(formattingRule, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetFormattingRuleHandler()
	})

	integrityRule := mcp.NewResource(
		"hive://rule/integrity",
		"Integrity Rule",
		mcp.WithResourceDescription("TheHive integrity rule"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(integrityRule, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetIntegrityRuleHandler()
	})

	filteringRule := mcp.NewResource(
		"hive://rule/filtering",
		"Filtering Rule",
		mcp.WithResourceDescription("TheHive filtering rule"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(filteringRule, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetFilteringRuleHandler()
	})
}
