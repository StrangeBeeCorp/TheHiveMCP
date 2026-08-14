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

// GetAlertSchemaHandler returns the alert output schema.
func GetAlertSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("alert/OutputAlert")
}

// GetAlertCreateSchemaHandler returns the alert create-input schema.
func GetAlertCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("alert/CreateAlert")
}

// GetAlertUpdateSchemaHandler returns the alert update-input schema.
func GetAlertUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("alert/UpdateAlert")
}

// GetCaseSchemaHandler returns the case output schema.
func GetCaseSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case/OutputCase")
}

// GetCaseCreateSchemaHandler returns the case create-input schema.
func GetCaseCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case/CreateCase")
}

// GetCaseUpdateSchemaHandler returns the case update-input schema.
func GetCaseUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case/UpdateCase")
}

// GetTaskSchemaHandler returns the task output schema.
func GetTaskSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("task/OutputTask")
}

// GetTaskCreateSchemaHandler returns the task create-input schema.
func GetTaskCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("task/CreateTask")
}

// GetTaskUpdateSchemaHandler returns the task update-input schema.
func GetTaskUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("task/UpdateTask")
}

// GetObservableSchemaHandler returns the observable output schema.
func GetObservableSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("observable/OutputObservable")
}

// GetObservableCreateSchemaHandler returns the observable create-input schema.
func GetObservableCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("observable/CreateObservable")
}

// GetObservableUpdateSchemaHandler returns the observable update-input schema.
func GetObservableUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("observable/UpdateObservable")
}

// GetProcedureSchemaHandler returns the procedure output schema.
func GetProcedureSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("ttp/OutputProcedure")
}

// GetProcedureCreateSchemaHandler returns the procedure create-input schema.
func GetProcedureCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("ttp/CreateProcedure")
}

// GetProcedureUpdateSchemaHandler returns the procedure update-input schema.
func GetProcedureUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("ttp/UpdateProcedure")
}

// GetPatternSchemaHandler returns the pattern output schema.
func GetPatternSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("ttp/OutputPattern")
}

// registerAutomationSchemaResources registers the Cortex job and action schemas.
// Kept out of RegisterSchemaResources, which is already at the limit of what one
// function should hold.
func registerAutomationSchemaResources(registry *ResourceRegistry) {
	jobSchema := mcp.NewResource(
		"hive://schema/job",
		"Analyzer Job Schema",
		mcp.WithResourceDescription("Output and filterable fields for Cortex analyzer jobs returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)

	registry.Register(jobSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetJobSchemaHandler()
	})

	actionSchema := mcp.NewResource(
		"hive://schema/action",
		"Responder Action Schema",
		mcp.WithResourceDescription("Output and filterable fields for Cortex responder actions returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)

	registry.Register(actionSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetActionSchemaHandler()
	})
}

// GetJobSchemaHandler returns the Cortex analyzer job output schema.
func GetJobSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("automation/OutputJob")
}

// GetActionSchemaHandler returns the Cortex responder action output schema.
func GetActionSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("automation/OutputAction")
}

// GetCaseTemplateSchemaHandler returns the case template output schema.
func GetCaseTemplateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case_template/OutputCaseTemplate")
}

// GetCaseTemplateCreateSchemaHandler returns the case template create-input schema.
func GetCaseTemplateCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case_template/CreateCaseTemplate")
}

// GetCaseTemplateUpdateSchemaHandler returns the case template update-input schema.
func GetCaseTemplateUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("case_template/UpdateCaseTemplate")
}

// GetPageSchemaHandler returns the page output schema.
func GetPageSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("page/OutputPage")
}

// GetPageCreateSchemaHandler returns the page create-input schema.
func GetPageCreateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("page/CreatePage")
}

// GetPageUpdateSchemaHandler returns the page update-input schema.
func GetPageUpdateSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("page/UpdatePage")
}

// GetFilterSchemaHandler returns the filter schema.
func GetFilterSchemaHandler() ([]mcp.ResourceContents, error) {
	return getSchemaContent("Filter")
}

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

// DateData carries the current date into the date fact template.
type DateData struct {
	CurrentDate string
}

// GetDateFactHandler returns the current server time as a fact resource.
func GetDateFactHandler() ([]mcp.ResourceContents, error) {
	data := DateData{
		CurrentDate: time.Now().Format("2006-01-02T15:04:05Z07:00"),
	}

	return getFactContent("date", data)
}

// GetHiveFactHandler returns the TheHive platform overview fact resource.
func GetHiveFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("thehive", nil)
}

// GetTaskFactHandler returns the task documentation fact resource.
func GetTaskFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent(types.EntityTypeTask, nil)
}

// GetObservableFactHandler returns the observable documentation fact resource.
func GetObservableFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent(types.EntityTypeObservable, nil)
}

// GetAlertFactHandler returns the alert documentation fact resource.
func GetAlertFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent(types.EntityTypeAlert, nil)
}

// GetCaseFactHandler returns the case documentation fact resource.
func GetCaseFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent(types.EntityTypeCase, nil)
}

// GetResponderFactHandler returns the responder documentation fact resource.
func GetResponderFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("responder", nil)
}

// GetAnalyzerFactHandler returns the analyzer documentation fact resource.
func GetAnalyzerFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent("analyzer", nil)
}

// GetProcedureFactHandler returns the procedure documentation fact resource.
func GetProcedureFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent(types.EntityTypeProcedure, nil)
}

// GetPatternFactHandler returns the pattern documentation fact resource.
func GetPatternFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent(types.EntityTypePattern, nil)
}

// GetCaseTemplateFactHandler returns the case template documentation fact resource.
func GetCaseTemplateFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent(types.EntityTypeCaseTemplate, nil)
}

// GetPageFactHandler returns the page documentation fact resource.
func GetPageFactHandler() ([]mcp.ResourceContents, error) {
	return getFactContent(types.EntityTypePage, nil)
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

// RegisterSchemaResources registers the entity schema resources on the registry.
func RegisterSchemaResources(registry *ResourceRegistry) {
	alertSchema := mcp.NewResource(
		"hive://schema/alert",
		"Alert Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for alerts returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(alertSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetAlertSchemaHandler()
	})

	alertCreateSchema := mcp.NewResource(
		"hive://schema/alert/create",
		"Alert Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new alerts"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(alertCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetAlertCreateSchemaHandler()
	})

	alertUpdateSchema := mcp.NewResource(
		"hive://schema/alert/update",
		"Alert Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing alerts"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(alertUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetAlertUpdateSchemaHandler()
	})

	caseSchema := mcp.NewResource(
		"hive://schema/case",
		"Case Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for cases returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(caseSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseSchemaHandler()
	})

	caseCreateSchema := mcp.NewResource(
		"hive://schema/case/create",
		"Case Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new cases"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(caseCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseCreateSchemaHandler()
	})

	caseUpdateSchema := mcp.NewResource(
		"hive://schema/case/update",
		"Case Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing cases"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(caseUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseUpdateSchemaHandler()
	})

	taskSchema := mcp.NewResource(
		"hive://schema/task",
		"Task Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for tasks returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(taskSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetTaskSchemaHandler()
	})

	taskCreateSchema := mcp.NewResource(
		"hive://schema/task/create",
		"Task Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new tasks"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(taskCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetTaskCreateSchemaHandler()
	})

	taskUpdateSchema := mcp.NewResource(
		"hive://schema/task/update",
		"Task Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing tasks"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(taskUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetTaskUpdateSchemaHandler()
	})

	observableSchema := mcp.NewResource(
		"hive://schema/observable",
		"Observable Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for observables returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(observableSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetObservableSchemaHandler()
	})

	observableCreateSchema := mcp.NewResource(
		"hive://schema/observable/create",
		"Observable Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new observables"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(observableCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetObservableCreateSchemaHandler()
	})

	observableUpdateSchema := mcp.NewResource(
		"hive://schema/observable/update",
		"Observable Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing observables"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(observableUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetObservableUpdateSchemaHandler()
	})

	procedureSchema := mcp.NewResource(
		"hive://schema/procedure",
		"Procedure Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for procedures returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)

	registry.Register(procedureSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetProcedureSchemaHandler()
	})

	procedureCreateSchema := mcp.NewResource(
		"hive://schema/procedure/create",
		"Procedure Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new procedures"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)

	registry.Register(procedureCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetProcedureCreateSchemaHandler()
	})

	procedureUpdateSchema := mcp.NewResource(
		"hive://schema/procedure/update",
		"Procedure Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing procedures"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(procedureUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetProcedureUpdateSchemaHandler()
	})

	patternSchema := mcp.NewResource(
		"hive://schema/pattern",
		"Pattern Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for patterns returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)

	registry.Register(patternSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetPatternSchemaHandler()
	})

	registerAutomationSchemaResources(registry)

	caseTemplateSchema := mcp.NewResource(
		uriCaseTemplateSchema,
		"Case Template Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for case templates"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(caseTemplateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseTemplateSchemaHandler()
	})

	caseTemplateCreateSchema := mcp.NewResource(
		uriCaseTemplateSchema+suffixCreate,
		"Case Template Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new case templates"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(caseTemplateCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseTemplateCreateSchemaHandler()
	})

	caseTemplateUpdateSchema := mcp.NewResource(
		uriCaseTemplateSchema+suffixUpdate,
		"Case Template Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing case templates"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(caseTemplateUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetCaseTemplateUpdateSchemaHandler()
	})

	pageSchema := mcp.NewResource(
		"hive://schema/page",
		"Page Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for pages returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(pageSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetPageSchemaHandler()
	})

	pageCreateSchema := mcp.NewResource(
		"hive://schema/page/create",
		"Page Create Schema",
		mcp.WithResourceDescription("Input fields and requirements for creating new pages"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(pageCreateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetPageCreateSchemaHandler()
	})

	pageUpdateSchema := mcp.NewResource(
		"hive://schema/page/update",
		"Page Update Schema",
		mcp.WithResourceDescription("Partial input fields for updating existing pages"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(pageUpdateSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetPageUpdateSchemaHandler()
	})

	filterSchema := mcp.NewResource(
		"hive://schema/filter",
		"Filter Schema",
		mcp.WithResourceDescription("TheHive filter data structure for search queries"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(filterSchema, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetFilterSchemaHandler()
	})
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

// RegisterFactResources registers the fact and documentation resources on the registry.
func RegisterFactResources(registry *ResourceRegistry) {
	serverTime := mcp.NewResource(
		"hive://config/server-time",
		"Server Time",
		mcp.WithResourceDescription("Current server date/time for timestamp calculations"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(serverTime, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetDateFactHandler()
	})

	theHiveOverview := mcp.NewResource(
		"hive://docs/overview/platform",
		"TheHive Overview",
		mcp.WithResourceDescription("General facts about TheHive platform, workflow, and capabilities"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(theHiveOverview, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetHiveFactHandler()
	})

	taskDocumentation := mcp.NewResource(
		"hive://docs/entities/task",
		"Task Documentation",
		mcp.WithResourceDescription("How tasks work, assignment, and task groups"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(taskDocumentation, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return GetTaskFactHandler()
	})

	observableDocumentation := mcp.NewResource(
		"hive://docs/entities/observable",
		"Observable Documentation",
		mcp.WithResourceDescription("How observables work, IOC types, enrichment workflow"),
		mcp.WithMIMEType(mimeApplicationJSON),
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
		mcp.WithMIMEType(mimeApplicationJSON),
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
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(
		caseDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetCaseFactHandler()
		},
	)

	procedureDocumentation := mcp.NewResource(
		"hive://docs/entities/procedure",
		"Procedure Documentation",
		mcp.WithResourceDescription("How procedures work, when to use them, and best practices"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(
		procedureDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetProcedureFactHandler()
		},
	)

	patternDocumentation := mcp.NewResource(
		"hive://docs/entities/pattern",
		"Pattern Documentation",
		mcp.WithResourceDescription("How patterns work, syntax, and usage in procedures"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(
		patternDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetPatternFactHandler()
		},
	)

	caseTemplateDocumentation := mcp.NewResource(
		uriCaseTemplateDocs,
		"Case Template Documentation",
		mcp.WithResourceDescription("How case templates work, their role as AI knowledge layer, and usage patterns"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(
		caseTemplateDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetCaseTemplateFactHandler()
		},
	)

	pageDocumentation := mcp.NewResource(
		"hive://docs/entities/page",
		"Page Documentation",
		mcp.WithResourceDescription("How pages work, their relationship to cases, and usage patterns"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(
		pageDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetPageFactHandler()
		},
	)

	analyzerDocumentation := mcp.NewResource(
		"hive://docs/automation/analyzers",
		"Analyzer Documentation",
		mcp.WithResourceDescription("How analyzers work, when to use them, and interpreting results"),
		mcp.WithMIMEType(mimeApplicationJSON),
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
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(
		responderDocumentation,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetResponderFactHandler()
		},
	)

	filterDslDoc := mcp.NewResource(
		"hive://docs/overview/filter-dsl",
		"Filter DSL Cheatsheet",
		mcp.WithResourceDescription("Operator grammar and worked examples for building TheHive search filters passed to the search-entities tool"),
		mcp.WithMIMEType("text/markdown"),
	)
	registry.Register(
		filterDslDoc,
		func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return GetFilterDslDocHandler()
		},
	)

	catalogResource := mcp.NewResource(
		"hive://catalog",
		"Resource Catalog",
		mcp.WithResourceDescription("Directory of all available resource categories and their purposes. This is the starting point for exploring resources."),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(catalogResource, GetResourceCatalog)
}

// RegisterStaticResources registers all static schema, rule, and fact resources.
func RegisterStaticResources(registry *ResourceRegistry) {
	RegisterSchemaResources(registry)
	RegisterRuleResources(registry)
	RegisterFactResources(registry)
}
