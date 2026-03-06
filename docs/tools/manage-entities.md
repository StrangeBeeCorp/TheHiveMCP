# manage-entities

Perform CRUD and workflow operations on TheHive entities (alerts, cases, tasks, observables, procedures).

## Overview

The `manage-entities` tool provides comprehensive Create, Read, Update, Delete, Comment, Promote, and Merge operations for all TheHive entity types. It allows you to manipulate entities programmatically while respecting TheHive's data integrity and relationship constraints.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `operation` | string | Yes | Operation to perform (`create`, `update`, `delete`, `comment`, `promote`, `merge`) |
| `entity-type` | string | Yes | Type of entity (`alert`, `case`, `task`, `observable`, `procedure`) |
| `entity-ids` | array | Conditional | List of entity IDs (usage varies by operation) |
| `entity-data` | object | Conditional | JSON object with entity data (required for create/update, optional for promote) |
| `comment` | string | Conditional | Text content (required for comment operations) |
| `target-id` | string | Conditional | Target entity ID (required for merge operations on alerts/observables) |

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

#### Creating procedures (requires parent case/alert)

A procedure maps observed attacker behaviour to a MITRE ATT&CK technique. Use `search-entities` with `entity-type="pattern"` to find the `patternId` before creating a procedure.

**Minimal example (required fields only):**
```json
{
  "operation": "create",
  "entity-type": "procedure",
  "entity-ids": ["case-123"],
  "entity-data": {
    "patternId": "T1059",
    "occurDate": "2024-01-15T10:30:00"
  }
}
```

**Recommended example (with tactic and description):**
```json
{
  "operation": "create",
  "entity-type": "procedure",
  "entity-ids": ["case-123"],
  "entity-data": {
    "patternId": "T1059.001",
    "occurDate": "2024-01-15T10:30:00",
    "tactic": "execution",
    "description": "Attacker executed PowerShell scripts to download and run malicious payloads"
  }
}
```

**Notes:**
- `patternId` must reference a valid MITRE ATT&CK technique loaded in TheHive (use `search-entities` with `entity-type="pattern"` to find valid IDs)
- `tactic` must be one of the tactics listed on the pattern (only required if the technique belongs to multiple tactics)
- `occurDate` is the timestamp when the attacker behaviour was observed
- Procedures can be attached to cases or alerts

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

Update an existing procedure (use the procedure's own ID, not the parent case/alert ID):
```json
{
  "operation": "update",
  "entity-type": "procedure",
  "entity-ids": ["~456"],
  "entity-data": {
    "description": "Updated analysis: attacker used PowerShell to download Cobalt Strike beacon",
    "occurDate": "2024-01-15T09:45:00"
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

### Promote operations

Convert an alert into a new case. The alert's observables, TTPs, and other data are transferred to the newly created case.

#### Promoting an alert to a case
```json
{
  "operation": "promote",
  "entity-type": "alert",
  "entity-ids": ["alert-123"]
}
```

#### Promoting with case creation parameters
```json
{
  "operation": "promote",
  "entity-type": "alert",
  "entity-ids": ["alert-123"],
  "entity-data": {
    "caseTemplate": "incident-response-template",
    "title": "Custom Case Title"
  }
}
```

**Notes:**
- Only alerts can be promoted
- Requires exactly one alert ID
- Optional `entity-data` can specify case creation parameters like `caseTemplate`
- Returns the newly created case

### Merge operations

Merge entities together. Behavior varies by entity type.

#### Merging cases together
Merges multiple cases into a single new case. All tasks, observables, and other data from the source cases are combined.

```json
{
  "operation": "merge",
  "entity-type": "case",
  "entity-ids": ["case-123", "case-456", "case-789"]
}
```

**Requirements:**
- Requires at least 2 case IDs in `entity-ids`
- Returns a single merged case containing all data from source cases

#### Merging alerts into a case
Merges one or more alerts into an existing case. The alerts' observables and data are added to the target case.

```json
{
  "operation": "merge",
  "entity-type": "alert",
  "entity-ids": ["alert-123", "alert-456"],
  "target-id": "case-789"
}
```

**Requirements:**
- Requires alert IDs in `entity-ids`
- Requires `target-id` specifying the case to merge alerts into
- The target case must exist

#### Deduplicating observables in a case
Merges similar observables within a case (deduplication). This finds and merges observables with identical data values.

```json
{
  "operation": "merge",
  "entity-type": "observable",
  "target-id": "case-123"
}
```

**Requirements:**
- Requires `target-id` specifying the case containing observables to deduplicate
- No `entity-ids` needed - operates on all similar observables in the case

## Entity Relationships and Constraints

### Hierarchical structure
- **Cases** are top-level entities
- **Tasks** belong to cases
- **Observables** can belong to cases OR alerts
- **Procedures** can belong to cases OR alerts
- **Alerts** are independent but can be promoted to cases

### Creation constraints
- **Tasks**: Must specify parent case ID in `entity-ids`
- **Observables**: Must specify parent case or alert ID in `entity-ids`
- **Procedures**: Must specify parent case or alert ID in `entity-ids`
- **Alerts**: Can be created independently
- **Cases**: Can be created independently

### Comment constraints
- **Cases**: Support standard comments
- **Tasks**: Use "task logs" instead of comments
- **Alerts**: Not supported for comments
- **Observables**: Not supported for comments
- **Procedures**: Not supported for comments

### Promote constraints
- **Alerts**: Can be promoted to cases
- **Cases, Tasks, Observables**: Not supported for promotion
- Requires exactly one alert ID

### Merge constraints
- **Cases**: Can be merged together (requires 2+ case IDs)
- **Alerts**: Can be merged into an existing case (requires target case ID)
- **Observables**: Can be deduplicated within a case (requires target case ID)
- **Tasks**: Not supported for merging

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
- `hive://schema/procedure/create` - Required and optional fields for creating procedures

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
- `hive://schema/procedure/update` - Fields available for updating procedures

### For understanding OUTPUT:
Available output schemas (for understanding query results):
- `hive://schema/alert` - Fields returned when querying alerts
- `hive://schema/case` - Fields returned when querying cases
- `hive://schema/task` - Fields returned when querying tasks
- `hive://schema/observable` - Fields returned when querying observables
- `hive://schema/procedure` - Fields returned when querying procedures
- `hive://schema/pattern` - Fields returned when querying patterns (MITRE ATT&CK techniques)

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
- `tlp`: Traffic Light Protocol (0-4, default varies by organisation)
- `pap`: Permissible Actions Protocol (0-3, default varies by organisation)
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
4. Map attacker behaviour to MITRE ATT&CK by creating procedures
5. Update case status as investigation progresses
6. Add comments to document findings

### TTP workflow
1. Search for relevant MITRE ATT&CK techniques: `search-entities` with `entity-type="pattern"` and a keyword query
2. Note the `patternId` and available `tactics` from the pattern
3. Create a procedure on the case or alert with the `patternId`, `occurDate`, and optionally `tactic` and `description`
4. Update or delete the procedure if details change during investigation

### Alert processing
1. Create alert from external source
2. Analyze alert content and create observables
3. Promote alert to case if investigation needed (use `promote` operation)
4. Create tasks for investigation activities
5. Merge related alerts into the case if more alerts arrive (use `merge` operation with alerts)

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

### Case consolidation
When multiple cases are related to the same incident:
1. Identify related cases through search or analysis
2. Merge cases together to consolidate all data (use `merge` operation with cases)
3. The merged case contains all tasks, observables, and comments from source cases
4. Continue investigation in the merged case

### Observable deduplication
After importing data or merging alerts:
1. Check for duplicate observables in a case
2. Use merge operation on observables to deduplicate (use `merge` operation with observable entity-type)
3. Identical observables are merged, keeping all relevant metadata

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
