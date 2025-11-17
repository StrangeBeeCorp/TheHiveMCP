# get-resource

Access TheHive resources for documentation, schemas, and metadata.

## Overview

The `get-resource` tool is the entry point for exploring TheHive's capabilities. It provides hierarchical access to documentation, schemas, and metadata through a URI-based resource system.

## Resource Structure

Resources are organized hierarchically:

- `hive://catalog` - Directory of all categories
- `hive://config/*` - Session and system info
- `hive://schema/*` - Entity field definitions
- `hive://metadata/*` - Available options and choices
- `hive://docs/*` - Documentation and guides

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `uri` | string | No | Full resource URI (e.g., 'hive://schema/alert'). Mutually exclusive with category. |
| `category` | string | No | Category to browse (e.g., 'schema', 'metadata', 'docs'). Mutually exclusive with uri. |

## Usage Patterns

### 1. Discovery Mode
Call without parameters to list all available categories:
```
get-resource()
```

### 2. Browse Mode
Provide a category to list resources within that category:
```
get-resource(category="schema")
get-resource(category="metadata")
get-resource(category="docs")
```

### 3. Fetch Mode
Provide a full URI to get a specific resource:
```
get-resource(uri="hive://schema/alert")
get-resource(uri="hive://docs/entities/case")
get-resource(uri="hive://metadata/automation/analyzers")
```

## Examples

- **List all categories**: `get-resource()`
- **List schemas**: `get-resource(category="schema")`
- **Get alert schema**: `get-resource(uri="hive://schema/alert")`
- **Get case documentation**: `get-resource(uri="hive://docs/entities/case")`
- **Get available analyzers**: `get-resource(uri="hive://metadata/automation/analyzers")`

## Best Practices

1. **Start with discovery**: Always begin by exploring the catalog to understand available resources
2. **Schema first**: Query entity schemas before creating or updating entities
3. **Documentation reference**: Use docs resources to understand entity relationships and workflows
4. **Metadata exploration**: Check metadata resources for available options and choices

## Integration with Other Tools

The `get-resource` tool is designed to work seamlessly with other MCP tools:

- Use it to understand schemas before calling `manage-entities`
- Reference documentation before using `search-entities`
- Check available analyzers/responders before using `execute-automation`

This tool ensures you have the most up-to-date information about TheHive's capabilities and can make informed decisions when interacting with the system.
