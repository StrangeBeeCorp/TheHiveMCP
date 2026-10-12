package manage

import (
	"github.com/StrangeBeeCorp/thehive4go/thehive"

	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
)

// CreateAlertResult is the tool result returned after creating an alert.
type CreateAlertResult struct {
	Operation  string               `json:"operation"`
	EntityType string               `json:"entityType"`
	Result     *FilteredOutputAlert `json:"result,omitempty"`
	Message    utils.TrustedString  `json:"message,omitempty"`
}

// NewCreateAlertResult builds a CreateAlertResult from a created OutputAlert.
func NewCreateAlertResult(alert *thehive.OutputAlert) *CreateAlertResult {
	return &CreateAlertResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeAlert,
		Result:     NewFilteredOutputAlert(alert),
		Message:    "Alert created successfully",
	}
}

// CreateCaseResult is the tool result returned after creating a case.
type CreateCaseResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	Result     *FilteredOutputCase `json:"result,omitempty"`
	Message    utils.TrustedString `json:"message,omitempty"`
}

// NewCreateCaseResult builds a CreateCaseResult from a created OutputCase.
func NewCreateCaseResult(caseEntity *thehive.OutputCase) *CreateCaseResult {
	return &CreateCaseResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeCase,
		Result:     NewFilteredOutputCase(caseEntity),
		Message:    "Case created successfully",
	}
}

// CreateTaskResult is the tool result returned after creating a task.
type CreateTaskResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	Result     *FilteredOutputTask `json:"result,omitempty"`
	Message    utils.TrustedString `json:"message,omitempty"`
}

// NewCreateTaskResult builds a CreateTaskResult from a created OutputTask.
func NewCreateTaskResult(task *thehive.OutputTask) *CreateTaskResult {
	return &CreateTaskResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeTask,
		Result:     NewFilteredOutputTask(task),
		Message:    "Task created successfully",
	}
}

// CreateObservableResult is the tool result returned after creating observables.
type CreateObservableResult struct {
	Operation  string                     `json:"operation"`
	EntityType string                     `json:"entityType"`
	Result     []FilteredOutputObservable `json:"result,omitempty"`
	// Failures lists the values TheHive refused when it created only some of
	// them; each entry is TheHive's error, with the refused value under object.
	Failures []utils.UntrustedSubtree `json:"failures,omitempty"`
	Message  utils.TrustedString      `json:"message,omitempty"`
}

// newPartialCreateObservableResult builds the result of a create in which
// TheHive made some observables and refused others.
func newPartialCreateObservableResult(created []FilteredOutputObservable, failures []utils.UntrustedSubtree) *CreateObservableResult {
	return &CreateObservableResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeObservable,
		Result:     created,
		Failures:   failures,
		Message: utils.Trustedf("Created %d of %d observables; TheHive refused %d, listed in failures. Do not create the ones in result again. "+
			"A failure of type CreateError is a value that already exists in this case or alert: it needs no retry.",
			len(created), len(created)+len(failures), len(failures)),
	}
}

// NewCreateObservableResult builds a CreateObservableResult from created OutputObservables.
func NewCreateObservableResult(observable []thehive.OutputObservable) *CreateObservableResult {
	filtered := make([]FilteredOutputObservable, len(observable))
	for i, o := range observable {
		filtered[i] = *NewFilteredOutputObservable(&o)
	}

	return &CreateObservableResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeObservable,
		Result:     filtered,
		Message:    "Observable created successfully",
	}
}

// CreateProcedureResult is the tool result returned after creating a procedure.
type CreateProcedureResult struct {
	Operation  string                  `json:"operation"`
	EntityType string                  `json:"entityType"`
	Result     FilteredOutputProcedure `json:"result"`
	Message    utils.TrustedString     `json:"message,omitempty"`
}

// NewCreateProcedureResult builds a CreateProcedureResult from a created OutputProcedure.
func NewCreateProcedureResult(procedure *thehive.OutputProcedure) *CreateProcedureResult {
	return &CreateProcedureResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeProcedure,
		Result:     *NewFilteredOutputProcedure(procedure),
		Message:    "Procedure created successfully",
	}
}

// SingleEntityUpdateResult is the per-entity outcome of an update operation.
type SingleEntityUpdateResult struct {
	EntityID string              `json:"_id"`
	Result   utils.TrustedString `json:"result,omitempty"`
	Error    map[string]any      `json:"error,omitempty"`
}

// UpdateEntityResult aggregates the per-entity outcomes of an update operation.
type UpdateEntityResult struct {
	Operation  string                     `json:"operation"`
	EntityType string                     `json:"entityType"`
	Results    []SingleEntityUpdateResult `json:"results,omitempty"`
}

// NewUpdateEntityResult builds an UpdateEntityResult for the given entity type and per-entity results.
func NewUpdateEntityResult(entityType string, results []SingleEntityUpdateResult) *UpdateEntityResult {
	return &UpdateEntityResult{
		Operation:  OperationUpdate,
		EntityType: entityType,
		Results:    results,
	}
}

// SingleEntityDeleteResult is the per-entity outcome of a delete operation.
type SingleEntityDeleteResult struct {
	EntityID string         `json:"_id"`
	Deleted  bool           `json:"deleted,omitempty"`
	Error    map[string]any `json:"error,omitempty"`
}

// DeleteEntityResult aggregates the per-entity outcomes of a delete operation.
type DeleteEntityResult struct {
	Operation  string                     `json:"operation"`
	EntityType string                     `json:"entityType"`
	Results    []SingleEntityDeleteResult `json:"results,omitempty"`
}

// NewDeleteEntityResult builds a DeleteEntityResult for the given entity type and per-entity results.
func NewDeleteEntityResult(entityType string, results []SingleEntityDeleteResult) *DeleteEntityResult {
	return &DeleteEntityResult{
		Operation:  OperationDelete,
		EntityType: entityType,
		Results:    results,
	}
}

// SingleEntityCommentResult is the per-entity outcome of a comment operation.
type SingleEntityCommentResult struct {
	CommentID string              `json:"commentId,omitempty"`
	EntityID  string              `json:"entityId"`
	Result    utils.TrustedString `json:"result,omitempty"`
	Error     map[string]any      `json:"error,omitempty"`
}

// CommentEntityResult aggregates the per-entity outcomes of a comment operation.
type CommentEntityResult struct {
	Operation  string                      `json:"operation"`
	EntityType string                      `json:"entityType"`
	Results    []SingleEntityCommentResult `json:"results,omitempty"`
}

// NewCommentEntityResult builds a CommentEntityResult for the given entity type and per-entity results.
func NewCommentEntityResult(entityType string, results []SingleEntityCommentResult) *CommentEntityResult {
	return &CommentEntityResult{
		Operation:  OperationComment,
		EntityType: entityType,
		Results:    results,
	}
}

// PromoteAlertResult is the tool result returned after promoting an alert to a case.
type PromoteAlertResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	Result     *FilteredOutputCase `json:"result,omitempty"`
}

// NewPromoteAlertResult builds a PromoteAlertResult from the created case.
func NewPromoteAlertResult(caseEntity *thehive.OutputCase) *PromoteAlertResult {
	return &PromoteAlertResult{
		Operation:  OperationPromote,
		EntityType: types.EntityTypeCase,
		Result:     NewFilteredOutputCase(caseEntity),
	}
}

// MergeCasesResult is the tool result returned after merging cases.
type MergeCasesResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	EntityIDs  []string            `json:"entityIds,omitempty"`
	Result     *FilteredOutputCase `json:"result,omitempty"`
	Message    utils.TrustedString `json:"message,omitempty"`
}

// NewMergeCasesResult builds a MergeCasesResult from the merged case and the source case IDs.
func NewMergeCasesResult(caseEntity *thehive.OutputCase, mergedIDs []string) *MergeCasesResult {
	return &MergeCasesResult{
		Operation:  OperationMerge,
		EntityType: types.EntityTypeCase,
		EntityIDs:  mergedIDs,
		Result:     NewFilteredOutputCase(caseEntity),
		Message:    "Cases merged successfully",
	}
}

// MergeAlertsIntoCaseResult is the tool result returned after merging alerts into a case.
type MergeAlertsIntoCaseResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	EntityIDs  []string            `json:"entityIds,omitempty"`
	TargetID   string              `json:"targetId,omitempty"`
	Result     *FilteredOutputCase `json:"result,omitempty"`
	Message    utils.TrustedString `json:"message,omitempty"`
}

// NewMergeAlertsResult builds a MergeAlertsIntoCaseResult from the target case, merged alert IDs, and target case ID.
func NewMergeAlertsResult(caseEntity *thehive.OutputCase, alertIDs []string, targetCaseID string) *MergeAlertsIntoCaseResult {
	return &MergeAlertsIntoCaseResult{
		Operation:  OperationMerge,
		EntityType: types.EntityTypeCase,
		EntityIDs:  alertIDs,
		TargetID:   targetCaseID,
		Result:     NewFilteredOutputCase(caseEntity),
		Message:    "Alerts merged into case successfully",
	}
}

// MergeObservablesResult is the tool result returned after merging/deduplicating observables.
type MergeObservablesResult struct {
	Operation  string `json:"operation"`
	EntityType string `json:"entityType"`
	TargetID   string `json:"targetId,omitempty"`
	// Result stays a plain string on purpose: it echoes the API merge summary
	// (upstream content) and must remain [UNTRUSTED_DATA]-wrapped.
	Result  string              `json:"result,omitempty"`
	Message utils.TrustedString `json:"message,omitempty"`
}

// NewMergeObservablesResult builds a MergeObservablesResult from the API summary and target case ID.
func NewMergeObservablesResult(resultData, targetCaseID string) *MergeObservablesResult {
	return &MergeObservablesResult{
		Operation:  OperationMerge,
		EntityType: types.EntityTypeObservable,
		TargetID:   targetCaseID,
		Result:     resultData,
		Message:    "Observables merged/deduplicated successfully",
	}
}

// EntityResult is the union result type for all manage-entities operations.
type EntityResult struct {
	CreateAlertResult        *CreateAlertResult         `json:"createAlertResult,omitempty"`
	CreateCaseResult         *CreateCaseResult          `json:"createCaseResult,omitempty"`
	CreateTaskResult         *CreateTaskResult          `json:"createTaskResult,omitempty"`
	CreateObservableResult   *CreateObservableResult    `json:"createObservableResult,omitempty"`
	CreateProcedureResult    *CreateProcedureResult     `json:"createProcedureResult,omitempty"`
	CreateCaseTemplateResult *CreateCaseTemplateResult  `json:"createCaseTemplateResult,omitempty"`
	CreatePageResult         *CreatePageResult          `json:"createPageResult,omitempty"`
	UpdateResults            *UpdateEntityResult        `json:"updateResults,omitempty"`
	DeleteResults            *DeleteEntityResult        `json:"deleteResults,omitempty"`
	CommentResults           *CommentEntityResult       `json:"commentResults,omitempty"`
	PromoteAlertResult       *PromoteAlertResult        `json:"promoteAlertResult,omitempty"`
	MergeCasesResult         *MergeCasesResult          `json:"mergeCasesResult,omitempty"`
	MergeAlertsResult        *MergeAlertsIntoCaseResult `json:"mergeAlertsResult,omitempty"`
	MergeObservablesResult   *MergeObservablesResult    `json:"mergeObservablesResult,omitempty"`
	ApplyTemplateResult      *ApplyTemplateResult       `json:"applyTemplateResult,omitempty"`
}

// Unwrap implements utils.Unwrapper to flatten the union for serialization.
func (r EntityResult) Unwrap() any { return utils.UnwrapUnion(r) }

// CreatePageResult is the tool result returned after creating a page.
type CreatePageResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	Result     *FilteredOutputPage `json:"result,omitempty"`
	Message    utils.TrustedString `json:"message,omitempty"`
}

// NewCreatePageResult builds a CreatePageResult from a created OutputPage.
func NewCreatePageResult(page *thehive.OutputPage) *CreatePageResult {
	return &CreatePageResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypePage,
		Result:     NewFilteredOutputPage(page),
		Message:    "Page created successfully",
	}
}

// CreateCaseTemplateResult is the tool result returned after creating a case template.
type CreateCaseTemplateResult struct {
	Operation  string                      `json:"operation"`
	EntityType string                      `json:"entityType"`
	Result     *FilteredOutputCaseTemplate `json:"result,omitempty"`
	Message    utils.TrustedString         `json:"message,omitempty"`
}

// NewCreateCaseTemplateResult builds a CreateCaseTemplateResult from a created OutputCaseTemplate.
func NewCreateCaseTemplateResult(ct *thehive.OutputCaseTemplate) *CreateCaseTemplateResult {
	return &CreateCaseTemplateResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeCaseTemplate,
		Result:     NewFilteredOutputCaseTemplate(ct),
		Message:    "Case template created successfully",
	}
}

// ApplyTemplateResult is the tool result returned after applying a case template to cases.
type ApplyTemplateResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	TemplateID string              `json:"templateId"`
	CaseIDs    []string            `json:"caseIds"`
	Message    utils.TrustedString `json:"message,omitempty"`
}

// NewApplyTemplateResult builds an ApplyTemplateResult from the template ID and target case IDs.
func NewApplyTemplateResult(templateID string, caseIDs []string) *ApplyTemplateResult {
	return &ApplyTemplateResult{
		Operation:  OperationApplyTemplate,
		EntityType: types.EntityTypeCaseTemplate,
		TemplateID: templateID,
		CaseIDs:    caseIDs,
		Message:    "Case template applied successfully",
	}
}
