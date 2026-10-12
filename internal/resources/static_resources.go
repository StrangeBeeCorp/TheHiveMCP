package resources

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

const (
	// mimeApplicationJSON is the MIME type served for JSON resource contents.
	mimeApplicationJSON = "application/json"
	// suffixCreate / suffixUpdate name the input-schema variants of an entity.
	suffixCreate = "/create"
	suffixUpdate = "/update"
	// keyName / keyDescription / keyResources are catalog map keys reused
	// across the resource catalog and registry listings.
	keyName        = "name"
	keyDescription = "description"
	keyResources   = "resources"
	// uriCaseTemplateSchema / uriCaseTemplateDocs are the resource URIs for the
	// case-template entity, reused across schema variants and documentation.
	uriCaseTemplateSchema = "hive://schema/case-template"
	uriCaseTemplateDocs   = "hive://docs/entities/case-template"
)

//go:embed schemas/*.json schemas/*/*.json
var schemasFS embed.FS

//go:embed rules/*.txt
var rulesFS embed.FS

//go:embed facts/*.txt
var factsFS embed.FS

//go:embed docs/*.md
var docsFS embed.FS

func getSchemaContent(schemaName string) ([]mcp.ResourceContents, error) {
	schemaBytes, err := schemasFS.ReadFile(fmt.Sprintf("schemas/%s.json", schemaName))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s schema: %w", schemaName, err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "docs://schema/" + schemaName,
			MIMEType: mimeApplicationJSON,
			Text:     string(schemaBytes),
		},
	}, nil
}

func getRuleContent(ruleName string) ([]mcp.ResourceContents, error) {
	ruleBytes, err := rulesFS.ReadFile(fmt.Sprintf("rules/%s.txt", ruleName))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s rule: %w", ruleName, err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "docs://rule/" + ruleName,
			MIMEType: "text/plain",
			Text:     string(ruleBytes),
		},
	}, nil
}

func getFactContent(factName string, data any) ([]mcp.ResourceContents, error) {
	factBytes, err := factsFS.ReadFile(fmt.Sprintf("facts/%s.txt", factName))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s fact: %w", factName, err)
	}

	tmpl, err := template.New(factName).Parse(string(factBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, data)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "docs://fact/" + factName,
			MIMEType: "text/plain",
			Text:     buf.String(),
		},
	}, nil
}

// GetFilterDslDocHandler returns the filter DSL cheatsheet document.
func GetFilterDslDocHandler() ([]mcp.ResourceContents, error) {
	docBytes, err := docsFS.ReadFile("docs/filter-dsl.md")
	if err != nil {
		return nil, fmt.Errorf("failed to read filter-dsl doc: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://docs/overview/filter-dsl",
			MIMEType: "text/markdown",
			Text:     string(docBytes),
		},
	}, nil
}

// GetCatalogData returns the resource catalog's category structure.
func GetCatalogData() map[string]any {
	return map[string]any{
		"categories": []map[string]any{
			{
				keyName:        "config",
				keyDescription: "Current session and system configuration. Includes authenticated user info and server time.",
				keyResources:   []string{"current-user", "server-time", "permissions"},
			},
			{
				keyName:        "schema",
				keyDescription: "Entity field definitions and data types. Query these to understand what fields are available for each entity type and their constraints. Each entity has three variants: base (output), /create (input for creation), and /update (partial input for updates).",
				keyResources: []string{
					types.EntityTypeAlert,
					types.EntityTypeAlert + suffixCreate,
					types.EntityTypeAlert + suffixUpdate,
					types.EntityTypeCase,
					types.EntityTypeCase + suffixCreate,
					types.EntityTypeCase + suffixUpdate,
					types.EntityTypeTask,
					types.EntityTypeTask + suffixCreate,
					types.EntityTypeTask + suffixUpdate,
					types.EntityTypeObservable,
					types.EntityTypeObservable + suffixCreate,
					types.EntityTypeObservable + suffixUpdate,
					types.EntityTypeProcedure,
					types.EntityTypeProcedure + suffixCreate,
					types.EntityTypeProcedure + suffixUpdate,
					types.EntityTypePattern,
					types.EntityTypeCaseTemplate,
					types.EntityTypeCaseTemplate + suffixCreate,
					types.EntityTypeCaseTemplate + suffixUpdate,
					types.EntityTypePage,
					types.EntityTypePage + suffixCreate,
					types.EntityTypePage + suffixUpdate,
					types.EntityTypeJob,
					types.EntityTypeAction,
					"filter",
				},
			},
			{
				keyName:        "metadata",
				keyDescription: "Available options, enumerations, and choices. Use these to get valid values for dropdowns, assignments, and entity properties.",
				"subcategories": []map[string]any{
					{
						keyName:        "entities",
						keyDescription: "Entity-specific metadata like statuses, templates, and types",
						keyResources:   []string{"case/statuses", "case/templates", "observable/types", "custom-fields"},
					},
					{
						keyName:        "automation",
						keyDescription: "Cortex integration resources for analyzers and responders",
						keyResources:   []string{"analyzers", "responders"},
					},
					{
						keyName:        "organisation",
						keyDescription: "Organisation settings and user management",
						keyResources:   []string{"users"},
					},
				},
			},
			{
				keyName:        "docs",
				keyDescription: "Documentation and educational content about TheHive platform, entities, and workflows. Read these to understand best practices.",
				"subcategories": []map[string]any{
					{
						keyName:        "overview",
						keyDescription: "Platform-wide documentation and general information",
						keyResources:   []string{"platform", "filter-dsl"},
					},
					{
						keyName:        "entities",
						keyDescription: "Entity-specific guides and best practices",
						keyResources:   []string{types.EntityTypeAlert, types.EntityTypeCase, types.EntityTypeTask, types.EntityTypeObservable, types.EntityTypeProcedure, types.EntityTypePattern, types.EntityTypeCaseTemplate, types.EntityTypePage},
					},
					{
						keyName:        "automation",
						keyDescription: "Automation workflow guides for analyzers and responders",
						keyResources:   []string{"analyzers", "responders"},
					},
				},
			},
		},
	}
}

// GetResourceCatalog returns the catalog (uses the same GetCatalogData)
func GetResourceCatalog(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	catalog := GetCatalogData()
	catalog["usage"] = map[string]any{
		"discover": "Use get-resource tool without parameters to list all categories",
		"browse":   "Use get-resource tool with a URI to browse a category (e.g., uri=\"hive://schema\" or uri=\"hive://metadata/automation\")",
		"fetch":    "Use get-resource tool with a URI to fetch a specific resource (e.g., uri=\"hive://schema/alert\")",
	}

	content, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal catalog: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://catalog",
			MIMEType: mimeApplicationJSON,
			Text:     string(content),
		},
	}, nil
}

// RegisterStaticResources registers all static schema, rule, and fact resources.
func RegisterStaticResources(registry *ResourceRegistry) {
	RegisterSchemaResources(registry)
	RegisterRuleResources(registry)
	RegisterFactResources(registry)
}
