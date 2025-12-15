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

### Create operations

Create new entities with complete schema data.

#### Creating alerts

**Minimal example (required fields only):**
```json
{
  "operation": "create",
  "entity-type": "alert",
  "entity-data": {
    "type": "external",
    "source": "SIEM",
    "sourceRef": "SIEM-2024-001234",
    "title": "Suspicious Network Activity",
    "description": "Detected unusual traffic patterns"
  }
}
```

**Recommended example (with common fields):**
```json
{
  "operation": "create",
  "entity-type": "alert",
  "entity-data": {
    "type": "external",
    "source": "SIEM",
    "sourceRef": "SIEM-2024-001234",
    "title": "Suspicious Network Activity",
    "description": "Detected unusual traffic patterns",
    "severity": 3,
    "tags": ["network", "suspicious"]
  }
}
```

**Complete example with optional fields:**
```json
{
  "operation": "create",
  "entity-type": "alert",
  "entity-data": {
    "type": "external",
    "source": "SIEM",
    "sourceRef": "SIEM-2024-001234",
    "title": "Suspicious Network Activity",
    "description": "Detected unusual traffic patterns from internal network segment",
    "severity": 3,
    "tlp": 2,
    "pap": 2,
    "tags": ["network", "suspicious", "internal"],
    "assignee": "analyst@example.com",
    "externalLink": "https://siem.company.com/alert/001234"
  }
}
```

#### Creating cases
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

#### Creating tasks (requires parent case)
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

#### Creating observables (requires parent case/alert)
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

### Update operations

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

### Delete operations

**⚠️ Warning**: Delete operations are irreversible!

```json
{
  "operation": "delete",
  "entity-type": "task",
  "entity-ids": ["task-456"]
}
```

### Comment operations

Add comments to cases or task logs to tasks.

#### Adding case comments
```json
{
  "operation": "comment",
  "entity-type": "case",
  "entity-ids": ["case-123"],
  "comment": "Found additional IOCs in network logs"
}
```

#### Adding task logs
```json
{
  "operation": "comment",
  "entity-type": "task",
  "entity-ids": ["task-456"],
  "comment": "Analysis completed - no malicious indicators found"
}
```

## Entity Relationships and Constraints

### Hierarchical structure
- **Cases** are top-level entities
- **Tasks** belong to cases
- **Observables** can belong to cases OR alerts
- **Alerts** are independent but can be promoted to cases

### Creation constraints
- **Tasks**: Must specify parent case ID in `entity-ids`
- **Observables**: Must specify parent case or alert ID in `entity-ids`
- **Alerts**: Can be created independently
- **Cases**: Can be created independently

### Comment constraints
- **Cases**: Support standard comments
- **Tasks**: Use "task logs" instead of comments
- **Alerts**: Not supported for comments
- **Observables**: Not supported for comments

## Schema Reference

Before creating or updating entities, always check the appropriate schema:

### For CREATE operations:
```json
{
  "tool": "get-resource",
  "uri": "hive://schema/alert/create"
}
```

Available create schemas:
- `hive://schema/alert/create` - Required and optional fields for creating alerts
- `hive://schema/case/create` - Required and optional fields for creating cases
- `hive://schema/task/create` - Required and optional fields for creating tasks
- `hive://schema/observable/create` - Required and optional fields for creating observables

### For UPDATE operations:
```json
{
  "tool": "get-resource",
  "uri": "hive://schema/alert/update"
}
```

Available update schemas:
- `hive://schema/alert/update` - Fields available for updating alerts
- `hive://schema/case/update` - Fields available for updating cases
- `hive://schema/task/update` - Fields available for updating tasks
- `hive://schema/observable/update` - Fields available for updating observables

### For understanding OUTPUT:
Available output schemas (for understanding query results):
- `hive://schema/alert` - Fields returned when querying alerts
- `hive://schema/case` - Fields returned when querying cases
- `hive://schema/task` - Fields returned when querying tasks
- `hive://schema/observable` - Fields returned when querying observables

## Best Practices

### Before creating entities
1. **Query schemas**: Use `get-resource` to understand required fields
2. **Check metadata**: Verify valid values for enums and choices
3. **Validate relationships**: Ensure parent entities exist

### Alert-specific requirements
When creating alerts, the following fields are **required**:
- `type`: Alert category (for example, "external", "malware", "phishing")
- `source`: Source system name (for example, "SIEM", "EDR", "Email Gateway")
- `sourceRef`: Unique reference from source system (for example, "SIEM-2024-001234")
- `title`: Brief alert summary
- `description`: Detailed alert description

**Highly recommended fields** (may have system defaults but should be explicitly set):
- `severity`: Numeric severity level (1-4, where 4 is most critical)

**Optional but commonly used fields**:
- `tlp`: Traffic Light Protocol (0-4, default varies by organization)
- `pap`: Permissible Actions Protocol (0-3, default varies by organization)
- `tags`: Array of classification tags
- `assignee`: Email/username to assign the alert
- `externalLink`: URL to view alert in source system
- `flag`: Boolean to mark the alert for attention
- `summary`: Brief triage notes or summary

### Data integrity
1. **Required fields**: Always include mandatory schema fields
2. **Data types**: Match expected types (string, number, array, etc.)
3. **Enum values**: Use valid enumeration values
4. **Relationships**: Maintain proper parent-child relationships

### Update operations
1. **Partial updates**: Only include fields that need to change
2. **Field validation**: Ensure new values meet schema constraints
3. **State transitions**: Follow valid status/stage transitions

### Security considerations
1. **Permissions**: Ensure user has appropriate permissions
2. **Data sensitivity**: Handle sensitive data appropriately
3. **Audit trail**: All operations are logged in TheHive

## Common Patterns

### Investigation workflow
1. Create case for investigation
2. Create tasks for specific activities
3. Create observables as evidence is collected
4. Update case status as investigation progresses
5. Add comments to document findings

### Alert processing
1. Create alert from external source
2. Analyze alert content and create observables
3. Promote to case if investigation needed
4. Create tasks for investigation activities

### Batch operations
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
