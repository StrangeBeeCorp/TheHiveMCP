package resources

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
)

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

	registry.Register(jobSchema, withFilterable(types.EntityTypeJob, GetJobSchemaHandler))

	actionSchema := mcp.NewResource(
		"hive://schema/action",
		"Responder Action Schema",
		mcp.WithResourceDescription("Output and filterable fields for Cortex responder actions returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)

	registry.Register(actionSchema, withFilterable(types.EntityTypeAction, GetActionSchemaHandler))
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

// RegisterSchemaResources registers the entity schema resources on the registry.
func RegisterSchemaResources(registry *ResourceRegistry) {
	alertSchema := mcp.NewResource(
		"hive://schema/alert",
		"Alert Output Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for alerts returned from TheHive API"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(alertSchema, withFilterable(types.EntityTypeAlert, GetAlertSchemaHandler))

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
	registry.Register(caseSchema, withFilterable(types.EntityTypeCase, GetCaseSchemaHandler))

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
	registry.Register(taskSchema, withFilterable(types.EntityTypeTask, GetTaskSchemaHandler))

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
	registry.Register(observableSchema, withFilterable(types.EntityTypeObservable, GetObservableSchemaHandler))

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

	registry.Register(procedureSchema, withFilterable(types.EntityTypeProcedure, GetProcedureSchemaHandler))

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

	registry.Register(patternSchema, withFilterable(types.EntityTypePattern, GetPatternSchemaHandler))

	registerAutomationSchemaResources(registry)

	caseTemplateSchema := mcp.NewResource(
		uriCaseTemplateSchema,
		"Case Template Schema",
		mcp.WithResourceDescription("Output fields, types, and constraints for case templates"),
		mcp.WithMIMEType(mimeApplicationJSON),
	)
	registry.Register(caseTemplateSchema, withFilterable(types.EntityTypeCaseTemplate, GetCaseTemplateSchemaHandler))

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
	registry.Register(pageSchema, withFilterable(types.EntityTypePage, GetPageSchemaHandler))

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
