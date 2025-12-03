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
| `uri` | string | No | Resource URI to query (e.g., 'hive://schema/alert', 'hive://metadata/automation'). Omit to list all categories. |

## Usage

The tool automatically determines whether you're requesting a specific resource or browsing a category based on the URI provided.

**URI Flexibility:**
- URIs work with or without the `hive://` prefix (e.g., `"schema"` or `"hive://schema"`)
- Trailing slashes are handled automatically (e.g., `"hive://schema/"` or `"hive://schema"`)

**Behavior:**
- If the URI points to a specific resource, the resource content is returned
- If the URI is a path, all resources and subcategories under that path are returned
- If the URI doesn't exist, an error is returned

### Discovery Mode
Call without parameters to list all available categories:
```
get-resource()
```

### Browse Mode
Provide a URI to browse resources and subcategories at that path:
```
get-resource(uri="hive://schema")
get-resource(uri="hive://metadata")
get-resource(uri="hive://metadata/automation")
get-resource(uri="hive://docs/entities")
```

### Fetch Mode
Provide a URI to get a specific resource:
```
get-resource(uri="hive://schema/alert")
get-resource(uri="hive://docs/entities/case")
get-resource(uri="hive://metadata/automation/analyzers")
```

## Examples

### Discovery and Browsing
- **List all categories**: `get-resource()`
- **List schemas**: `get-resource(uri="hive://schema")`
- **Browse automation metadata**: `get-resource(uri="hive://metadata/automation")`
- **Browse entity docs**: `get-resource(uri="hive://docs/entities")`

### Specific Resource Fetching
- **Get alert output schema**: `get-resource(uri="hive://schema/alert")`
- **Get alert create schema**: `get-resource(uri="hive://schema/alert/create")`
- **Get alert update schema**: `get-resource(uri="hive://schema/alert/update")`
- **Get case documentation**: `get-resource(uri="hive://docs/entities/case")`
- **Get available analyzers**: `get-resource(uri="hive://metadata/automation/analyzers")`

## Schema Organization

Entity schemas are organized into three variants:

- **Output schemas** (`hive://schema/{entity}`): Fields returned from TheHive API when querying entities
- **Create schemas** (`hive://schema/{entity}/create`): Required and optional fields for creating new entities
- **Update schemas** (`hive://schema/{entity}/update`): Partial fields available for updating existing entities

Available entities: `alert`, `case`, `task`, `observable`

Example:
- `hive://schema/task` - Output schema for tasks (what you get from queries)
- `hive://schema/task/create` - Input schema for creating tasks
- `hive://schema/task/update` - Partial input schema for updating tasks

This organization makes it clear which fields are required for creation vs available for updates.


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
