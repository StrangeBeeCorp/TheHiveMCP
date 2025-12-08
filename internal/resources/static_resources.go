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

//go:embed schemas/*.json schemas/*/*.json
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

// Alert schema handlers
func GetAlertSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("alert/OutputAlert")
}

func GetAlertCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("alert/CreateAlert")
}

func GetAlertUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("alert/UpdateAlert")
}

// Case schema handlers
func GetCaseSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case/OutputCase")
}

func GetCaseCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case/CreateCase")
}

func GetCaseUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case/UpdateCase")
}

// Task schema handlers
func GetTaskSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("task/OutputTask")
}

func GetTaskCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("task/CreateTask")
}

func GetTaskUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("task/UpdateTask")
}

// Observable schema handlers
func GetObservableSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("observable/OutputObservable")
}

func GetObservableCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("observable/CreateObservable")
}

func GetObservableUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("observable/UpdateObservable")
}

// Case template schema handler
func GetCaseTemplateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case_template/OutputCaseTemplate")
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
				"resources":   []string{"current-user", "server-time", "permissions"},
			},
			{
				"name":        "schema",
				"description": "Entity field definitions and data types. Query these to understand what fields are available for each entity type and their constraints. Each entity has three variants: base (output), /create (input for creation), and /update (partial input for updates).",
				"resources":   []string{"alert", "alert/create", "alert/update", "case", "case/create", "case/update", "task", "task/create", "task/update", "observable", "observable/create", "observable/update", "case-template", "filter"},
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
	catalog["usage"] = map[string]interface{}{
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
			MIMEType: "application/json",
			Text:     string(content),
		},
	}, nil
}

func RegisterSchemaResources(registry *ResourceRegistry) {
	// Alert schemas
	alertSchema := mcp.NewResource(
		"hive://schema/alert",
		"Alert Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for alerts returned from TheHive API"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(alertSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetAlertSchemaHandler()
	})

	alertCreateSchema := mcp.NewResource(
		"hive://schema/alert/create",
		"Alert Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new alerts"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(alertCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetAlertCreateSchemaHandler()
	})

	alertUpdateSchema := mcp.NewResource(
		"hive://schema/alert/update",
		"Alert Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing alerts"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(alertUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetAlertUpdateSchemaHandler()
	})

	// Case schemas
	caseSchema := mcp.NewResource(
		"hive://schema/case",
		"Case Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for cases returned from TheHive API"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(caseSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseSchemaHandler()
	})

	caseCreateSchema := mcp.NewResource(
		"hive://schema/case/create",
		"Case Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new cases"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(caseCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseCreateSchemaHandler()
	})

	caseUpdateSchema := mcp.NewResource(
		"hive://schema/case/update",
		"Case Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing cases"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(caseUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseUpdateSchemaHandler()
	})

	// Task schemas
	taskSchema := mcp.NewResource(
		"hive://schema/task",
		"Task Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for tasks returned from TheHive API"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(taskSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetTaskSchemaHandler()
	})

	taskCreateSchema := mcp.NewResource(
		"hive://schema/task/create",
		"Task Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new tasks"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(taskCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetTaskCreateSchemaHandler()
	})

	taskUpdateSchema := mcp.NewResource(
		"hive://schema/task/update",
		"Task Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing tasks"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(taskUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetTaskUpdateSchemaHandler()
	})

	// Observable schemas
	observableSchema := mcp.NewResource(
		"hive://schema/observable",
		"Observable Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for observables returned from TheHive API"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(observableSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetObservableSchemaHandler()
	})

	observableCreateSchema := mcp.NewResource(
		"hive://schema/observable/create",
		"Observable Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new observables"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(observableCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetObservableCreateSchemaHandler()
	})

	observableUpdateSchema := mcp.NewResource(
		"hive://schema/observable/update",
		"Observable Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing observables"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(observableUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetObservableUpdateSchemaHandler()
	})

	// Case template schema
	caseTemplateSchema := mcp.NewResource(
		"hive://schema/case-template",
		"Case Template Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for case templates"),
		mcp.WithMIMEType("application/json"),
	)
	registry.Register(caseTemplateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseTemplateSchemaHandler()
	})

	// Filter schema
	filterSchema := mcp.NewResource(
		"hive://schema/filter",
		"Filter Schema",
		mcp.WithResourceDescription("TheHive filter data structure for search queries"),
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
