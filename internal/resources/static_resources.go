package resources

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

//go:embed schemas/*.json
var schemasFS embed.FS

//go:embed rules/*.txt
var rulesFS embed.FS

//go:embed facts/*.txt
var factsFS embed.FS

// Update the file reading functions
func getSchemaContent(schemaName string) ([]mcp.ResourceContents, error) {
	schemaBytes, err := schemasFS.ReadFile(fmt.Sprintf("schemas/%s.json", schemaName))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s schema: %w", schemaName, err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      fmt.Sprintf("docs://schema/%s", schemaName),
			MIMEType: "application/json",
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
			URI:      fmt.Sprintf("docs://rule/%s", ruleName),
			MIMEType: "text/plain",
			Text:     string(ruleBytes),
		},
	}, nil
}

func getFactContent(factName string, data interface{}) ([]mcp.ResourceContents, error) {
	factBytes, err := factsFS.ReadFile(fmt.Sprintf("facts/%s.txt", factName))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s fact: %w", factName, err)
	}

	// Create a new template
	tmpl, err := template.New(factName).Parse(string(factBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute template with data
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      fmt.Sprintf("docs://fact/%s", factName),
			MIMEType: "text/plain",
			Text:     buf.String(),
		},
	}, nil
}

func GetAlertSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("OutputAlert")
}

func GetCaseSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("OutputCase")
}

func GetTaskSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("OutputTask")
}

func GetObservableSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("OutputObservable")
}

func GetFilterSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("Filter")
}

func GetFormattingRuleHandler() ([]mcp.ResourceContents, error) {
	return getRuleContent("formatting")
}

func GetIntegrityRuleHandler() ([]mcp.ResourceContents, error) {
	return getRuleContent("integrity")
}

func GetFilteringRuleHandler() ([]mcp.ResourceContents, error) {
	return getRuleContent("filtering")
}

type DateData struct {
	CurrentDate string
}

func GetDateFactHandler() ([]mcp.ResourceContents, error) {
	data := DateData{
		CurrentDate: time.Now().Format("2006-01-02T15:04:05Z07:00"),
	}
	return getFactContent("date", data)
}

func GetHiveFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("thehive", nil)
}

func GetTaskFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("task", nil)
}

func GetObservableFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("observable", nil)
}

func GetAlertFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("alert", nil)
}

func GetCaseFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("case", nil)
}

func GetResponderFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("responder", nil)
}

func GetAnalyzerFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("analyzer", nil)
}

// Helper to get catalog structure
func GetCatalogData() map[string]interface{} {
	return map[string]interface{}{
		"categories": []map[string]interface{}{
			{
				"name":        "config",
				"description": "Current session and system configuration. Includes authenticated user info and server time.",
				"resources":   []string{"current-user", "server-time"},
			},
			{
				"name":        "schema",
				"description": "Entity field definitions and data types. Query these to understand what fields are available for each entity type and their constraints.",
				"resources":   []string{"alert", "case", "task", "observable"},
			},
			{
				"name":        "metadata",
				"description": "Available options, enumerations, and choices. Use these to get valid values for dropdowns, assignments, and entity properties.",
				"subcategories": []map[string]interface{}{
					{
						"name":        "entities",
						"description": "Entity-specific metadata like statuses, templates, and types",
						"resources":   []string{"case/statuses", "case/templates", "observable/types", "custom-fields"},
					},
					{
						"name":        "automation",
						"description": "Cortex integration resources for analyzers and responders",
						"resources":   []string{"analyzers", "responders"},
					},
					{
						"name":        "organization",
						"description": "Organization settings and user management",
						"resources":   []string{"users"},
					},
				},
			},
			{
				"name":        "docs",
				"description": "Documentation and educational content about TheHive platform, entities, and workflows. Read these to understand best practices.",
				"subcategories": []map[string]interface{}{
					{
						"name":        "overview",
						"description": "Platform-wide documentation and general information",
						"resources":   []string{"platform"},
					},
					{
						"name":        "entities",
						"description": "Entity-specific guides and best practices",
						"resources":   []string{"alert", "case", "task", "observable"},
					},
					{
						"name":        "automation",
						"description": "Automation workflow guides for analyzers and responders",
						"resources":   []string{"analyzers", "responders"},
					},
				},
			},
		},
	}
}

// GetResourceCatalog returns the catalog (uses the same GetCatalogData)
func GetResourceCatalog(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	catalog := GetCatalogData()
	catalog["usage"] = map[string]string{
		"discover": "Use get-resource tool without parameters to list all categories",
		"browse":   "Use get-resource tool with category to list resources in that category",
		"fetch":    "Use get-resource tool with full URI to fetch specific resource",
	}

	content, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal catalog: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "hive://catalog",
			MIMEType: "application/json",
			Text:     string(content),
		},
	}, nil
}

func RegisterSchemaResources(registry *ResourceRegistry) {
	alertSchema := mcp.NewResource(
		"hive://schema/alert",
		"Alert Schema",
		mcp.WithResourceDescription("Available fields, types, and constraints for alerts"),
	)
	registry.Register(alertSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetAlertSchemaHandler()
	})

	caseSchema := mcp.NewResource(
		"hive://schema/case",
		"Case Schema",
		mcp.WithResourceDescription("Available fields, types, and constraints for cases"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(caseSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseSchemaHandler()
	})

	taskSchema := mcp.NewResource(
		"hive://schema/task",
		"Task Schema",
		mcp.WithResourceDescription("Available fields, types, and constraints for tasks"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(taskSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetTaskSchemaHandler()
	})

	observableSchema := mcp.NewResource(
		"hive://schema/observable",
		"Observable Schema",
		mcp.WithResourceDescription("Available fields, types, and constraints for observables"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(observableSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetObservableSchemaHandler()
	})

	filterSchema := mcp.NewResource(
		"hive://schema/filter",
		"Filter Schema",
		mcp.WithResourceDescription("TheHive filter data structure"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(filterSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetFilterSchemaHandler()
	})
}

func RegisterRuleResources(registry *ResourceRegistry) {
	formattingRule := mcp.NewResource(
		"hive://rule/formatting",
		"Formatting Rule",
		mcp.WithResourceDescription("TheHive formatting rule"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(formattingRule, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetFormattingRuleHandler()
	})
	integrityRule := mcp.NewResource(
		"hive://rule/integrity",
		"Integrity Rule",
		mcp.WithResourceDescription("TheHive integrity rule"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(integrityRule, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetIntegrityRuleHandler()
	})
	filteringRule := mcp.NewResource(
		"hive://rule/filtering",
		"Filtering Rule",
		mcp.WithResourceDescription("TheHive filtering rule"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(filteringRule, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetFilteringRuleHandler()
	})
}

func RegisterFactResources(registry *ResourceRegistry) {
	serverTime := mcp.NewResource(
		"hive://config/server-time",
		"Server Time",
		mcp.WithResourceDescription("Current server date/time for timestamp calculations"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(serverTime, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetDateFactHandler()
	})
	theHiveOverview := mcp.NewResource(
		"hive://docs/overview/platform",
		"TheHive Overview",
		mcp.WithResourceDescription("General facts about TheHive platform, workflow, and capabilities"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(theHiveOverview, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetHiveFactHandler()
	})
	taskDocumentation := mcp.NewResource(
		"hive://docs/entities/task",
		"Task Documentation",
		mcp.WithResourceDescription("How tasks work, assignment, and task groups"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(taskDocumentation, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetTaskFactHandler()
	})

	observableDocumentation := mcp.NewResource(
		"hive://docs/entities/observable",
		"Observable Documentation",
		mcp.WithResourceDescription("How observables work, IOC types, enrichment workflow"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(
		observableDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetObservableFactHandler()
		},
	)
	alertDocumentation := mcp.NewResource(
		"hive://docs/entities/alert",
		"Alert Documentation",
		mcp.WithResourceDescription("How alerts work, lifecycle, and best practices"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(
		alertDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetAlertFactHandler()
		},
	)
	caseDocumentation := mcp.NewResource(
		"hive://docs/entities/case",
		"Case Documentation",
		mcp.WithResourceDescription("How cases work, investigation workflow, TLP/PAP usage"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(
		caseDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetCaseFactHandler()
		},
	)

	analyzerDocumentation := mcp.NewResource(
		"hive://docs/automation/analyzers",
		"Analyzer Documentation",
		mcp.WithResourceDescription("How analyzers work, when to use them, and interpreting results"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(
		analyzerDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetAnalyzerFactHandler()
		},
	)
	responderDocumentation := mcp.NewResource(
		"hive://docs/automation/responders",
		"Responder Documentation",
		mcp.WithResourceDescription("How responders work, active response workflow, and PAP considerations"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(
		responderDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetResponderFactHandler()
		},
	)

	// Register resource catalog
	catalogResource := mcp.NewResource(
		"hive://catalog",
		"Resource Catalog",
		mcp.WithResourceDescription("Directory of all available resource categories and their purposes. This is the starting point for exploring resources."),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(
		catalogResource,
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetResourceCatalog(ctx, req)
		},
	)
}

func RegisterStaticResources(registry *ResourceRegistry) {
	RegisterSchemaResources(registry)
	RegisterRuleResources(registry)
	RegisterFactResources(registry)
}
