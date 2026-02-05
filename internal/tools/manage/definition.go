package manage

import (
	"github.com/StrangeBeeCorp/TheHiveMCP/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

type ManageTool struct{}

func NewManageTool() *ManageTool {
	return &ManageTool{}
}

func (t *ManageTool) Definition() mcp.Tool {
	return mcp.NewTool(
		"manage-entities",
		mcp.WithDescription(`Perform CRUD and workflow operations on TheHive entities (alerts, cases, tasks, observables).

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
- Dedupe observables: operation="merge", entity-type="observable", target-id="~123"`),
		mcp.WithString(
			"operation",
			mcp.Required(),
			mcp.Enum("create", "update", "delete", "comment", "promote", "merge"),
			mcp.Description("The operation to perform: create, update, delete, comment, promote, or merge."),
		),
		mcp.WithString(
			"entity-type",
			mcp.Required(),
			mcp.Enum(types.EntityTypeAlert, types.EntityTypeCase, types.EntityTypeTask, types.EntityTypeObservable),
			mcp.Description("Type of entity to manage."),
		),
		mcp.WithArray(
			"entity-ids",
			mcp.Description("List of entity IDs. Usage varies by operation: UPDATE/DELETE/COMMENT: entities to modify. CREATE (task/observable): parent case/alert ID. PROMOTE: single alert ID. MERGE (case): case IDs to merge. MERGE (alert): alert IDs to merge into target case."),
		),
		mcp.WithObject(
			"entity-data",
			mcp.Description("JSON object containing entity data. For CREATE: use get-resource hive://schema/[entity]/create for required fields. For UPDATE: only provide fields to change. For PROMOTE: optional case creation parameters."),
		),
		mcp.WithString(
			"comment",
			mcp.Description("Text content for COMMENT operations. Required when operation=\"comment\". For cases: adds a comment. For tasks: adds a task log entry."),
		),
		mcp.WithString(
			"target-id",
			mcp.Description("Target entity ID for MERGE operations. For alerts: the case ID to merge alerts into. For observables: the case ID containing observables to deduplicate."),
		),
	)
}
