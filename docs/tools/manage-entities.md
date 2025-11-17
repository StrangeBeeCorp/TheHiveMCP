# manage-entities

Perform CRUD operations on TheHive entities (alerts, cases, tasks, observables).

## Overview

The `manage-entities` tool provides comprehensive Create, Read, Update, Delete, and Comment operations for all TheHive entity types. It allows you to manipulate entities programmatically while respecting TheHive's data integrity and relationship constraints.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `operation` | string | Yes | Operation to perform (`create`, `update`, `delete`, `comment`) |
| `entity-type` | string | Yes | Type of entity (`alert`, `case`, `task`, `observable`) |
| `entity-ids` | array | Conditional | List of entity IDs (required for update/delete/comment operations) |
| `entity-data` | object | Conditional | JSON object with entity data (required for create/update) |
| `comment` | string | Conditional | Text content (required for comment operations) |

## Operations

### Create Operations

Create new entities with complete schema data.

#### Creating Alerts
```json
{
  "operation": "create",
  "entity-type": "alert",
  "entity-data": {
    "type": "external",
    "source": "SIEM",
    "title": "Suspicious Network Activity",
    "description": "Detected unusual traffic patterns",
    "severity": 3,
    "tags": ["network", "suspicious"]
  }
}
```

#### Creating Cases
```json
{
  "operation": "create",
  "entity-type": "case",
  "entity-data": {
    "title": "Phishing Investigation",
    "description": "Investigation of reported phishing email",
    "severity": 2,
    "assignee": "analyst@example.com",
    "tags": ["phishing", "email"]
  }
}
```

#### Creating Tasks (requires parent case)
```json
{
  "operation": "create",
  "entity-type": "task",
  "entity-ids": ["case-123"],
  "entity-data": {
    "title": "Analyze Email Headers",
    "description": "Extract and analyze email metadata",
    "assignee": "analyst@example.com"
  }
}
```

#### Creating Observables (requires parent case/alert)
```json
{
  "operation": "create",
  "entity-type": "observable",
  "entity-ids": ["case-123"],
  "entity-data": {
    "dataType": "ip",
    "data": "192.168.1.100",
    "message": "Suspicious IP from network logs",
    "tags": ["malicious", "network"]
  }
}
```

### Update Operations

Update existing entities with partial field changes.

```json
{
  "operation": "update",
  "entity-type": "case",
  "entity-ids": ["case-123"],
  "entity-data": {
    "status": "InProgress",
    "assignee": "senior-analyst@example.com",
    "severity": 3
  }
}
```

### Delete Operations

**⚠️ Warning**: Delete operations are irreversible!

```json
{
  "operation": "delete",
  "entity-type": "task",
  "entity-ids": ["task-456"]
}
```

### Comment Operations

Add comments to cases or task logs to tasks.

#### Adding Case Comments
```json
{
  "operation": "comment",
  "entity-type": "case",
  "entity-ids": ["case-123"],
  "comment": "Found additional IOCs in network logs"
}
```

#### Adding Task Logs
```json
{
  "operation": "comment",
  "entity-type": "task",
  "entity-ids": ["task-456"],
  "comment": "Analysis completed - no malicious indicators found"
}
```

## Entity Relationships and Constraints

### Hierarchical Structure
- **Cases** are top-level entities
- **Tasks** belong to cases
- **Observables** can belong to cases OR alerts
- **Alerts** are independent but can be promoted to cases

### Creation Constraints
- **Tasks**: Must specify parent case ID in `entity-ids`
- **Observables**: Must specify parent case or alert ID in `entity-ids`
- **Alerts**: Can be created independently
- **Cases**: Can be created independently

### Comment Constraints
- **Cases**: Support standard comments
- **Tasks**: Use "task logs" instead of comments
- **Alerts**: Not supported for comments
- **Observables**: Not supported for comments

## Schema Reference

Before creating or updating entities, always check the schema:

```json
{
  "tool": "get-resource",
  "uri": "hive://schema/alert"
}
```

Available schemas:
- `hive://schema/alert`
- `hive://schema/case`
- `hive://schema/task`
- `hive://schema/observable`

## Best Practices

### Before Creating Entities
1. **Query schemas**: Use `get-resource` to understand required fields
2. **Check metadata**: Verify valid values for enums and choices
3. **Validate relationships**: Ensure parent entities exist

### Data Integrity
1. **Required fields**: Always include mandatory schema fields
2. **Data types**: Match expected types (string, number, array, etc.)
3. **Enum values**: Use valid enumeration values
4. **Relationships**: Maintain proper parent-child relationships

### Update Operations
1. **Partial updates**: Only include fields that need to change
2. **Field validation**: Ensure new values meet schema constraints
3. **State transitions**: Follow valid status/stage transitions

### Security Considerations
1. **Permissions**: Ensure user has appropriate permissions
2. **Data sensitivity**: Handle sensitive data appropriately
3. **Audit trail**: All operations are logged in TheHive

## Common Patterns

### Investigation Workflow
1. Create case for investigation
2. Create tasks for specific activities
3. Create observables as evidence is collected
4. Update case status as investigation progresses
5. Add comments to document findings

### Alert Processing
1. Create alert from external source
2. Analyze alert content and create observables
3. Promote to case if investigation needed
4. Create tasks for investigation activities

### Batch Operations
Use multiple `entity-ids` for bulk operations:
```json
{
  "operation": "update",
  "entity-type": "task",
  "entity-ids": ["task-1", "task-2", "task-3"],
  "entity-data": {
    "status": "Completed"
  }
}
```

## Error Handling

Common errors and solutions:

- **Missing required fields**: Check schema and include all mandatory fields
- **Invalid parent ID**: Verify parent entity exists and is accessible
- **Permission denied**: Ensure user has appropriate permissions
- **Invalid field values**: Check metadata for valid enum values
- **Relationship constraints**: Verify entity relationships are valid

## Integration with Other Tools

- Use `search-entities` to find entities to manage
- Use `get-resource` to understand schemas and metadata
- Use `execute-automation` on created observables
- Reference entity IDs in other operations
