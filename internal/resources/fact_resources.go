package resources

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

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
