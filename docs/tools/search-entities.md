# search-entities

Search for entities in TheHive using natural language queries.

## Overview

The `search-entities` tool allows you to search for TheHive entities (alerts, cases, tasks, observables) using natural language queries. The tool uses AI to translate your natural language into TheHive filters, making it easy to find exactly what you're looking for without knowing the complex filter syntax.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `entity-type` | string | Yes | Type of entity to search for (`alert`, `case`, `task`, `observable`) |
| `query` | string | Yes | Natural language query describing what entities you want to find |
| `sort-by` | string | No | Column to sort results by (default: `_createdAt`) |
| `sort-order` | string | No | Sort order `asc` or `desc` (default: `desc`) |
| `limit` | number | No | Number of results to return (default: 10) |
| `extra-columns` | array | No | Additional columns to include in output (default: `['_id', 'title']`) |
| `extra-data` | array | No | Additional data fields to include in output |
| `additional-queries` | array | No | Additional queries to enrich results with related data |

## Natural Language Query Examples

The search tool understands various natural language patterns:

### Severity and Priority
- "high severity alerts from last week"
- "critical cases opened today"
- "medium priority alerts"

### Time-based Queries
- "alerts from the last month"
- "cases created yesterday"
- "tasks updated this week"
- "observables added in the last 24 hours"

### Status and Assignment
- "open cases assigned to john@example.com"
- "closed alerts"
- "waiting tasks"
- "in-progress cases"

### Content and Tags
- "observables containing malware"
- "phishing alerts"
- "cases tagged with APT"
- "tasks with keyword 'investigation'"

### Complex Queries
- "latest phishing alerts with severity greater than 2"
- "open cases with unassigned tasks"
- "malware observables from compromised systems"

## Supported Entity Types

### Alerts
Search for security alerts with filters on:
- Type, source, severity
- Tags and keywords
- Creation and update dates
- Status and assignee

### Cases
Search for investigation cases with filters on:
- Title, description, severity
- Status, stage, assignee
- Tags and custom fields
- Creation and resolution dates

### Tasks
Search for case tasks with filters on:
- Title, description, status
- Assignee and group
- Due dates and completion
- Task logs and updates

### Observables
Search for artifacts and IOCs with filters on:
- Data type and value
- Tags and analysis results
- Creation and update dates
- Associated cases or alerts

## Advanced Usage

### Custom Columns
Specify which fields to return in the results:
```json
{
  "entity-type": "case",
  "query": "high severity cases",
  "extra-columns": ["_id", "title", "severity", "assignee", "status"]
}
```

### Additional Data
Include extra data fields:
```json
{
  "entity-type": "alert",
  "query": "phishing alerts",
  "extra-data": ["tags", "case", "customFields"]
}
```

### Related Data Queries
Enrich results with related information:
```json
{
  "entity-type": "case",
  "query": "open investigations",
  "additional-queries": ["tasks", "observables"]
}
```

## Best Practices

1. **Be specific**: More specific queries yield better results
2. **Check schemas**: Use `get-resource` with output schemas (e.g., `hive://schema/alert`) to understand available fields for searching
3. **Review filters**: The tool returns the generated filters for transparency
4. **Iterate**: Refine your query based on results and filter feedback
5. **Limit results**: Use appropriate limits for performance

## Understanding Schema Types

When using search-entities, you'll work with output schemas to understand what fields are available for searching and what data will be returned:

- Use `hive://schema/alert` to see all fields available in alert search results
- Use `hive://schema/case` to see all fields available in case search results
- Use `hive://schema/task` to see all fields available in task search results
- Use `hive://schema/observable` to see all fields available in observable search results

For creating or updating entities found through search, use the create/update schema variants:
- `hive://schema/{entity}/create` for creating new entities
- `hive://schema/{entity}/update` for updating existing entities

## Query Understanding

The search tool understands:
- **Severity levels**: high, medium, low, critical
- **Status values**: open, closed, waiting, in-progress
- **Time references**: last week, yesterday, today, last month
- **Operators**: greater than, less than, contains, equals
- **Sorting**: latest, oldest, newest

## Integration Tips

- Start investigations with broad searches, then narrow down
- Use results to identify patterns and trends
- Combine with `get-resource` to understand entity relationships
- Use `manage-entities` to act on search results
- Export results for reporting and analysis

## Troubleshooting

If results don't match expectations:
1. Check the generated filters in the response
2. Review entity schemas using `get-resource`
3. Simplify the query to test individual conditions
4. Use more specific field names and values
