# manage-entities

Perform CRUD and workflow operations on TheHive entities (alerts, cases, tasks, observables, procedures, case templates, pages).

## Overview

Mutating and destructive: `delete` and `merge` are irreversible. See [Tool annotations](annotations.md).

The `manage-entities` tool provides comprehensive Create, Read, Update, Delete, Comment, Promote, and Merge operations for all TheHive entity types. It allows
you to manipulate entities programmatically while respecting TheHive's data integrity and relationship constraints.

## Parameters

| Parameter     | Type   | Required    | Description                                                                                          |
| ------------- | ------ | ----------- | ---------------------------------------------------------------------------------------------------- |
| `operation`   | string | Yes         | Operation to perform (`create`, `update`, `delete`, `comment`, `promote`, `merge`, `apply-template`) |
| `entity-type` | string | Yes         | Type of entity (`alert`, `case`, `task`, `observable`, `procedure`, `case-template`, `page`)         |
| `entity-ids`  | array  | Conditional | List of entity IDs (usage varies by operation)                                                       |
| `entity-data` | object | Conditional | JSON object with entity data (required for create/update, optional for promote)                      |
| `comment`     | string | Conditional | Text content (required for comment operations)                                                       |
| `target-id`   | string | Conditional | Target entity ID (required for merge operations on alerts/observables)                               |

Entity IDs in `entity-ids` and `target-id` are TheHive internal identifiers in `~`-prefixed numeric form (for example, `~123`) — the same `_id` returned by
[`search-entities`](search-entities.md). A case's human-readable `number` (for example, "case #42") is not an entity ID. Resolve it to an `_id` with a search
first.

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
  "entity-ids": ["~123"],
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
  "entity-ids": ["~123"],
  "entity-data": {
    "dataType": "ip",
    "data": "192.168.1.100",
    "message": "Suspicious IP from network logs",
    "tags": ["malicious", "network"]
  }
}
```

#### Creating procedures (requires parent case/alert)

A procedure maps observed attacker behaviour to a MITRE ATT&CK technique. Use `search-entities` with `entity-type="pattern"` to find the `patternId` before
creating a procedure.

**Minimal example (required fields only):**

```json
{
  "operation": "create",
  "entity-type": "procedure",
  "entity-ids": ["~123"],
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
  "entity-ids": ["~123"],
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

#### Creating pages

Pages can be created within a case or as standalone knowledge base articles.

**Creating a page in a case:**

```json
{
  "operation": "create",
  "entity-type": "page",
  "entity-ids": ["~123"],
  "entity-data": {
    "title": "Investigation Notes",
    "content": "## Summary\nInitial findings from the investigation...",
    "category": "Default"
  }
}
```

**Creating a standalone page (no parent case):**

```json
{
  "operation": "create",
  "entity-type": "page",
  "entity-data": {
    "title": "Incident Response Runbook",
    "content": "## Procedure\n1. Identify scope\n2. Contain threat\n3. Eradicate...",
    "category": "Default"
  }
}
```

**Notes:**

- `title`, `content`, and `category` are required fields
- `content` supports Markdown formatting
- If `entity-ids` is provided with a case ID, the page is created within that case
- If `entity-ids` is omitted, a standalone organisation-level page is created
- `order` is optional and controls display position (lower numbers appear first)

### Update operations

Update existing entities with partial field changes.

```json
{
  "operation": "update",
  "entity-type": "case",
  "entity-ids": ["~123"],
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

Update an existing page (use the page ID):

```json
{
  "operation": "update",
  "entity-type": "page",
  "entity-ids": ["~789"],
  "entity-data": {
    "title": "Updated Investigation Notes",
    "content": "## Updated Summary\nNew findings added..."
  }
}
```

### Delete operations

**⚠️ Warning**: Delete operations are irreversible!

```json
{
  "operation": "delete",
  "entity-type": "task",
  "entity-ids": ["~301"]
}
```

Delete a page:

```json
{
  "operation": "delete",
  "entity-type": "page",
  "entity-ids": ["~789"]
}
```

### Comment operations

Add comments to cases or task logs to tasks.

#### Adding case comments

```json
{
  "operation": "comment",
  "entity-type": "case",
  "entity-ids": ["~123"],
  "comment": "Found additional IOCs in network logs"
}
```

#### Adding task logs

```json
{
  "operation": "comment",
  "entity-type": "task",
  "entity-ids": ["~301"],
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
  "entity-ids": ["~201"]
}
```

#### Promoting with case creation parameters

```json
{
  "operation": "promote",
  "entity-type": "alert",
  "entity-ids": ["~201"],
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
  "entity-ids": ["~123", "~456", "~789"]
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
  "entity-ids": ["~201", "~202"],
  "target-id": "~789"
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
  "target-id": "~123"
}
```

**Requirements:**

- Requires `target-id` specifying the case containing observables to deduplicate
- No `entity-ids` needed - operates on all similar observables in the case

### Apply-template operations

Apply a case template to one or more existing cases. `target-id` is the case template name or ID. `entity-ids` are the cases to apply it to.

```json
{
  "operation": "apply-template",
  "entity-type": "case",
  "entity-ids": ["~123", "~456"],
  "target-id": "Phishing"
}
```

`entity-data` optionally selects which parts of the template to apply. Omit it to apply the template's defaults.

```json
{
  "operation": "apply-template",
  "entity-type": "case",
  "entity-ids": ["~123"],
  "target-id": "Phishing",
  "entity-data": {
    "updateDescription": true,
    "updateTags": true,
    "importTasks": ["Analyze headers", "Check sender reputation"]
  }
}
```

**Requirements:**

- Only supported for cases — use `entity-type="case"`.
- `entity-ids` (the target cases) and `target-id` (the template name or ID) are both required.
- Optional `entity-data` fields: `updateTitlePrefix`, `updateDescription`, `updateTags`, `updateSeverity`, `updateFlag`, `updateTlp`, `updatePap`,
  `updateCustomFields`, `importTasks`, `importPages`.

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

### For CREATE operations

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

### For UPDATE operations

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

### For understanding OUTPUT

Available output schemas (for understanding query results):

- `hive://schema/alert` - Fields returned when querying alerts
- `hive://schema/case` - Fields returned when querying cases
- `hive://schema/task` - Fields returned when querying tasks
- `hive://schema/observable` - Fields returned when querying observables
- `hive://schema/procedure` - Fields returned when querying procedures
- `hive://schema/pattern` - Fields returned when querying patterns (MITRE ATT&CK techniques)

## Batch operations

An operation accepts multiple `entity-ids` and applies to each:

```json
{
  "operation": "update",
  "entity-type": "task",
  "entity-ids": ["~301", "~302", "~303"],
  "entity-data": {
    "status": "Completed"
  }
}
```

## Notes

- **Field definitions.** Required, optional, and updatable fields per entity are defined by the schema resources, not this page — read `hive://schema/{entity}`
  and its `/create` and `/update` variants (via [`get-resource`](get-resource.md)) before building `entity-data`. Filtering an unknown or wrong-typed field
  fails.
- **Permissions.** Every operation is subject to the deployment's permission profile (tool allow/deny, `entity_permissions`, and scope filters). An entity
  outside a configured filter is reported as "not found or not within the scope" and nothing is mutated. See the [permissions reference](../permissions.md).
- **Auditing.** All operations are logged by TheHive.

For task-oriented walkthroughs (running an investigation, mapping TTPs), see the [first-investigation tutorial](../../tutorial/first-investigation.md). Find
entities to act on with [`search-entities`](search-entities.md), and enrich observables with [`execute-automation`](execute-automation.md).
