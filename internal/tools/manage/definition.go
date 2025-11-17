package manage

import (
	"github.com/mark3labs/mcp-go/mcp"
)

type ManageTool struct{}

func NewManageTool() *ManageTool {
	return &ManageTool{}
}

func (t *ManageTool) Definition() mcp.Tool {
	return mcp.NewTool(
		"manage-entities",
		mcp.WithDescription(`Perform CRUD operations on TheHive entities (alerts, cases, tasks, observables).

SUPPORTED OPERATIONS:
- CREATE: Create new entities with complete schema data
- UPDATE: Update existing entities by ID with partial field updates
- DELETE: Delete entities by ID (irreversible)
- COMMENT: Add comments to cases or task logs to tasks

IMPORTANT CONSTRAINTS:
- Tasks can only be created within a case (provide case ID in entity-ids parameter)
- Observables can be created in cases OR alerts (provide case or alert ID in entity-ids parameter)
- Comments are only supported on cases and tasks (tasks use 'task logs')
- DELETE operations are irreversible - use with caution

GETTING SCHEMA INFORMATION:
Use the get-resource tool to query schemas before creating/updating entities:
- get-resource: hive://schema/alert (for alert field definitions)
- get-resource: hive://schema/case (for case field definitions)
- get-resource: hive://schema/task (for task field definitions)
- get-resource: hive://schema/observable (for observable field definitions)

EXAMPLES:
- Create alert: operation="create", entity-type="alert", entity-data={"type":"...", "source":"...", "title":"..."}
- Update case: operation="update", entity-type="case", entity-ids=["case-id"], entity-data={"title":"New Title"}
- Add comment: operation="comment", entity-type="case", entity-ids=["case-id"], comment="Investigation update"`),
		mcp.WithString(
			"operation",
			mcp.Required(),
			mcp.Enum("create", "update", "delete", "comment"),
			mcp.Description("The operation to perform: create, update, delete, or comment."),
		),
		mcp.WithString(
			"entity-type",
			mcp.Required(),
			mcp.Enum("alert", "case", "task", "observable"),
			mcp.Description("Type of entity to manage."),
		),
		mcp.WithArray(
			"entity-ids",
			mcp.Description("List of entity IDs for UPDATE/DELETE/COMMENT operations. For CREATE: tasks and observables require parent case/alert ID. Example: ['alert-123', 'alert-456']"),
		),
		mcp.WithObject(
			"entity-data",
			mcp.Description("JSON object containing entity data. For CREATE: complete schema (use get-resource hive://schema/[entity] for required fields). For UPDATE: only fields to change. For task/observable creation: entity-ids should contain the parent case/alert ID."),
		),
		mcp.WithString(
			"comment",
			mcp.Description("Text content for COMMENT operations. Required when operation=\"comment\". For cases: adds a comment. For tasks: adds a task log entry."),
		),
	)
}
