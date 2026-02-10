package manage

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/utils"
	"github.com/StrangeBeeCorp/thehive4go/thehive"
)

const ManageToolDescription = `Perform CRUD and workflow operations on TheHive entities (alerts, cases, tasks, observables).

SUPPORTED OPERATIONS:
- CREATE: Create new entities with complete schema data
- UPDATE: Update existing entities by ID with partial field updates
- DELETE: Delete entities by ID (irreversible)
- COMMENT: Add comments to cases or task logs to tasks
- PROMOTE: Convert an alert into a new case (alert only)
- MERGE: Merge entities together (cases, alerts, observables)

IMPORTANT CONSTRAINTS:
- Tasks can only be created within a case (provide case ID in entity-ids parameter)
- Observables can be created in cases OR alerts (provide case or alert ID in entity-ids parameter)
- Comments are only supported on cases and tasks (tasks use 'task logs')
- DELETE operations are irreversible - use with caution
- PROMOTE only applies to alerts
- MERGE behavior varies by entity type (see below)

PROMOTE OPERATION (alert only):
Converts an alert into a new case. The alert's observables, TTPs, and other data are transferred to the case.
- Requires: entity-type="alert", entity-ids=[alert-id]
- Optional: entity-data with case creation parameters (template, title override, etc.)

MERGE OPERATION:
- For cases: Merges multiple cases into a single new case. Requires entity-ids with 2+ case IDs.
- For alerts: Merges alert(s) into an existing case. Requires entity-ids=[alert-ids...] and target-id=case-id.
- For observables: Merges similar observables within a case (deduplication). Requires target-id=case-id.

GETTING SCHEMA INFORMATION:
Use the get-resource tool to query schemas before creating/updating entities:
- Output schemas: hive://schema/alert, hive://schema/case, hive://schema/task, hive://schema/observable
- Create schemas: hive://schema/alert/create, hive://schema/case/create, hive://schema/task/create, hive://schema/observable/create
- Update schemas: hive://schema/alert/update, hive://schema/case/update, hive://schema/task/update, hive://schema/observable/update

EXAMPLES:
- Create alert: operation="create", entity-type="alert", entity-data={"type":"...", "source":"...", "title":"..."}
- Update case: operation="update", entity-type="case", entity-ids=["~123"], entity-data={"title":"New Title"}
- Add comment: operation="comment", entity-type="case", entity-ids=["~123"], comment="Investigation update"
- Promote alert: operation="promote", entity-type="alert", entity-ids=["~456"]
- Merge cases: operation="merge", entity-type="case", entity-ids=["~123", "~456"]
- Merge alert into case: operation="merge", entity-type="alert", entity-ids=["~789"], target-id="~123"
- Dedupe observables: operation="merge", entity-type="observable", target-id="~123"`

type ManageEntityParams struct {
	Operation  string                 `json:"operation" jsonschema:"enum=create,enum=update,enum=delete,enum=comment,enum=promote,enum=merge,required=true" jsonschema_description:"The operation to perform on the entity."`
	EntityType string                 `json:"entity-type" jsonschema:"enum=case,enum=alert,enum=task,enum=observable,required=true" jsonschema_description:"The type of entity to manage."`
	EntityIDs  []string               `json:"entity-ids,omitempty" jsonschema_description:"List of entity IDs. Usage varies by operation: UPDATE/DELETE/COMMENT: entities to modify. CREATE (task/observable): parent case/alert ID. PROMOTE: single alert ID. MERGE (case): case IDs to merge. MERGE (alert): alert IDs to merge into target case."`
	EntityData map[string]interface{} `json:"entity-data,omitempty" jsonschema_description:"JSON object containing entity data. For CREATE: use get-resource hive://schema/[entity]/create for required fields. For UPDATE: only provide fields to change. For PROMOTE: optional case creation parameters."`
	Comment    string                 `json:"comment,omitempty" jsonschema_description:"Text content for COMMENT operations. Required when operation=\"comment\". For cases: adds a comment. For tasks: adds a task log entry."`
	TargetID   string                 `json:"target-id,omitempty" jsonschema_description:"Target entity ID for MERGE operations. For alerts: the case ID to merge alerts into. For observables: the case ID containing observables to deduplicate."`
}

const (
	OperationCreate  = "create"
	OperationUpdate  = "update"
	OperationDelete  = "delete"
	OperationComment = "comment"
	OperationPromote = "promote"
	OperationMerge   = "merge"
)

type FilteredOutputAlert struct {
	UnderscoreId string `json:"_id"`
	Title        string `json:"title"`
	CreatedAt    int64  `json:"_createdAt"`
	Severity     int32  `json:"severity"`
	Status       string `json:"status"`
}

func NewFilteredOutputAlert(alert *thehive.OutputAlert) *FilteredOutputAlert {
	return &FilteredOutputAlert{
		UnderscoreId: alert.UnderscoreId,
		Title:        alert.Title,
		CreatedAt:    alert.UnderscoreCreatedAt,
		Severity:     alert.Severity,
		Status:       alert.Status,
	}
}

type CreateAlertResult struct {
	Operation  string               `json:"operation"`
	EntityType string               `json:"entityType"`
	Result     *FilteredOutputAlert `json:"result,omitempty"`
	Message    string               `json:"message,omitempty"`
}

func NewCreateAlertResult(alert *thehive.OutputAlert) *CreateAlertResult {
	return &CreateAlertResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeAlert,
		Result:     NewFilteredOutputAlert(alert),
		Message:    "Alert created successfully",
	}
}

type FilteredOutputCase struct {
	UnderscoreId string `json:"_id"`
	Title        string `json:"title"`
	CreatedAt    int64  `json:"_createdAt"`
	Status       string `json:"status"`
	Severity     int32  `json:"severity"`
}

func NewFilteredOutputCase(caseEntity *thehive.OutputCase) *FilteredOutputCase {
	return &FilteredOutputCase{
		UnderscoreId: caseEntity.UnderscoreId,
		Title:        caseEntity.Title,
		CreatedAt:    caseEntity.UnderscoreCreatedAt,
		Status:       caseEntity.Status,
		Severity:     caseEntity.Severity,
	}
}

type CreateCaseResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	Result     *FilteredOutputCase `json:"result,omitempty"`
	Message    string              `json:"message,omitempty"`
}

func NewCreateCaseResult(caseEntity *thehive.OutputCase) *CreateCaseResult {
	return &CreateCaseResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeCase,
		Result:     NewFilteredOutputCase(caseEntity),
		Message:    "Case created successfully",
	}
}

type FilteredOutputTask struct {
	UnderscoreId string  `json:"_id"`
	Title        string  `json:"title"`
	Status       string  `json:"status"`
	CreatedAt    int64   `json:"_createdAt"`
	Assignee     *string `json:"assignee,omitempty"`
}

func NewFilteredOutputTask(task *thehive.OutputTask) *FilteredOutputTask {
	return &FilteredOutputTask{
		UnderscoreId: task.UnderscoreId,
		Title:        task.Title,
		Status:       task.Status,
		CreatedAt:    task.UnderscoreCreatedAt,
		Assignee:     task.Assignee,
	}
}

type CreateTaskResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	Result     *FilteredOutputTask `json:"result,omitempty"`
	Message    string              `json:"message,omitempty"`
}

func NewCreateTaskResult(task *thehive.OutputTask) *CreateTaskResult {
	return &CreateTaskResult{
		Operation:  OperationCreate,
		EntityType: types.EntityTypeTask,
		Result:     NewFilteredOutputTask(task),
		Message:    "Task created successfully",
	}
}

type FilteredOutputObservable struct {
	UnderscoreId string `json:"_id"`
	DataType     string `json:"dataType"`
	CreatedAt    int64  `json:"_createdAt"`
}

func NewFilteredOutputObservable(observable *thehive.OutputObservable) *FilteredOutputObservable {
	return &FilteredOutputObservable{
		UnderscoreId: observable.UnderscoreId,
		DataType:     observable.DataType,
		CreatedAt:    observable.UnderscoreCreatedAt,
	}
}

type CreateObservableResult struct {
	Operation  string                     `json:"operation"`
	EntityType string                     `json:"entityType"`
	Result     []FilteredOutputObservable `json:"result,omitempty"`
	Message    string                     `json:"message,omitempty"`
}

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

type SingleEntityUpdateResult struct {
	EntityID string `json:"_id"`
	Result   string `json:"result,omitempty"`
	Error    string `json:"error,omitempty"`
}

type UpdateEntityResult struct {
	Operation  string                     `json:"operation"`
	EntityType string                     `json:"entityType"`
	Results    []SingleEntityUpdateResult `json:"results,omitempty"`
	Message    string                     `json:"message,omitempty"`
}

func NewUpdateEntityResult(entityType string, results []SingleEntityUpdateResult) *UpdateEntityResult {
	return &UpdateEntityResult{
		Operation:  OperationUpdate,
		EntityType: entityType,
		Results:    results,
		Message:    "Entity updated successfully",
	}
}

type SingleEntityDeleteResult struct {
	EntityID string `json:"_id"`
	Deleted  bool   `json:"deleted,omitempty"`
	Error    string `json:"error,omitempty"`
}

type DeleteEntityResult struct {
	Operation  string                     `json:"operation"`
	EntityType string                     `json:"entityType"`
	Results    []SingleEntityDeleteResult `json:"results,omitempty"`
	Message    string                     `json:"message,omitempty"`
}

func NewDeleteEntityResult(entityType string, results []SingleEntityDeleteResult) *DeleteEntityResult {
	return &DeleteEntityResult{
		Operation:  OperationDelete,
		EntityType: entityType,
		Results:    results,
		Message:    "Entity deletion completed",
	}
}

type SingleEntityCommentResult struct {
	CommentID string `json:"commentId,omitempty"`
	EntityID  string `json:"entityId"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

type CommentEntityResult struct {
	Operation  string                      `json:"operation"`
	EntityType string                      `json:"entityType"`
	Results    []SingleEntityCommentResult `json:"results,omitempty"`
	Message    string                      `json:"message,omitempty"`
}

func NewCommentEntityResult(entityType string, results []SingleEntityCommentResult) *CommentEntityResult {
	return &CommentEntityResult{
		Operation:  OperationComment,
		EntityType: entityType,
		Results:    results,
		Message:    "Comments added successfully",
	}
}

type PromoteAlertResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	Result     *FilteredOutputCase `json:"result,omitempty"`
	Message    string              `json:"message,omitempty"`
}

func NewPromoteAlertResult(caseEntity *thehive.OutputCase) *PromoteAlertResult {
	return &PromoteAlertResult{
		Operation:  OperationPromote,
		EntityType: types.EntityTypeCase,
		Result:     NewFilteredOutputCase(caseEntity),
		Message:    "Alert promoted to case successfully",
	}
}

type MergeCasesResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	EntityIds  []string            `json:"entityIds,omitempty"`
	Result     *FilteredOutputCase `json:"result,omitempty"`
	Message    string              `json:"message,omitempty"`
}

func NewMergeCasesResult(caseEntity *thehive.OutputCase, mergedIds []string) *MergeCasesResult {
	return &MergeCasesResult{
		Operation:  OperationMerge,
		EntityType: types.EntityTypeCase,
		EntityIds:  mergedIds,
		Result:     NewFilteredOutputCase(caseEntity),
		Message:    "Cases merged successfully",
	}
}

type MergeAlertsIntoCaseResult struct {
	Operation  string              `json:"operation"`
	EntityType string              `json:"entityType"`
	EntityIds  []string            `json:"entityIds,omitempty"`
	TargetId   string              `json:"targetId,omitempty"`
	Result     *FilteredOutputCase `json:"result,omitempty"`
	Message    string              `json:"message,omitempty"`
}

func NewMergeAlertsResult(caseEntity *thehive.OutputCase, alertIds []string, targetCaseId string) *MergeAlertsIntoCaseResult {
	return &MergeAlertsIntoCaseResult{
		Operation:  OperationMerge,
		EntityType: types.EntityTypeCase,
		EntityIds:  alertIds,
		TargetId:   targetCaseId,
		Result:     NewFilteredOutputCase(caseEntity),
		Message:    "Alerts merged into case successfully",
	}
}

type MergeObservablesResult struct {
	Operation  string `json:"operation"`
	EntityType string `json:"entityType"`
	TargetId   string `json:"targetId,omitempty"`
	Result     string `json:"result,omitempty"`
	Message    string `json:"message,omitempty"`
}

func NewMergeObservablesResult(resultData, targetCaseId string) *MergeObservablesResult {
	return &MergeObservablesResult{
		Operation:  OperationMerge,
		EntityType: types.EntityTypeObservable,
		TargetId:   targetCaseId,
		Result:     resultData,
		Message:    "Observables merged/deduplicated successfully",
	}
}

type ManageEntityResult struct {
	CreateAlertResult      *CreateAlertResult         `json:"createAlertResult,omitempty"`
	CreateCaseResult       *CreateCaseResult          `json:"createCaseResult,omitempty"`
	CreateTaskResult       *CreateTaskResult          `json:"createTaskResult,omitempty"`
	CreateObservableResult *CreateObservableResult    `json:"createObservableResult,omitempty"`
	UpdateResults          *UpdateEntityResult        `json:"updateResults,omitempty"`
	DeleteResults          *DeleteEntityResult        `json:"deleteResults,omitempty"`
	CommentResults         *CommentEntityResult       `json:"commentResults,omitempty"`
	PromoteAlertResult     *PromoteAlertResult        `json:"promoteAlertResult,omitempty"`
	MergeCasesResult       *MergeCasesResult          `json:"mergeCasesResult,omitempty"`
	MergeAlertsResult      *MergeAlertsIntoCaseResult `json:"mergeAlertsResult,omitempty"`
	MergeObservablesResult *MergeObservablesResult    `json:"mergeObservablesResult,omitempty"`
}

// Unwrap implements utils.Unwrapper to flatten the union for serialization.
func (r ManageEntityResult) Unwrap() any { return utils.UnwrapUnion(r) }
