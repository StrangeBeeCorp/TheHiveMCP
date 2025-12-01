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
| `uri` | string | No | Resource URI to query (e.g., 'hive://schema/alert', 'metadata/automation', or 'schema'). Works with or without 'hive://' prefix and with or without trailing slash. Omit to list all categories. |

## Usage

The tool automatically determines whether you're requesting a specific resource or browsing a category based on the URI provided. URIs work identically with or without trailing slashes, and the 'hive://' prefix is optional.

### Discovery Mode
Call without parameters to list all available categories:
```
get-resource()
```

### Browse Mode
Provide a URI to browse resources and subcategories at that path:
```
get-resource(uri="schema")
get-resource(uri="metadata")
get-resource(uri="metadata/automation")
get-resource(uri="docs/entities")
```

You can also use the full URI format with or without trailing slashes:
```
get-resource(uri="hive://schema")
get-resource(uri="hive://metadata/")
get-resource(uri="hive://metadata/automation")
```

### Fetch Mode
Provide a URI to get a specific resource:
```
get-resource(uri="hive://schema/alert")
get-resource(uri="docs/entities/case")
get-resource(uri="metadata/automation/analyzers")
```

## Examples

### Discovery and Browsing
- **List all categories**: `get-resource()`
- **List schemas**: `get-resource(uri="schema")`
- **Browse automation metadata**: `get-resource(uri="metadata/automation")`
- **Browse entity docs**: `get-resource(uri="docs/entities")`

### Specific Resource Fetching
- **Get alert output schema**: `get-resource(uri="hive://schema/alert")`
- **Get alert create schema**: `get-resource(uri="schema/alert/create")`
- **Get alert update schema**: `get-resource(uri="schema/alert/update")`
- **Get case documentation**: `get-resource(uri="docs/entities/case")`
- **Get available analyzers**: `get-resource(uri="metadata/automation/analyzers")`

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

## Flexible URI Format

The tool accepts URIs in multiple formats for convenience:

- **Full URI**: `hive://schema/alert`
- **Relative path**: `schema/alert`
- **With trailing slash**: `metadata/automation/`
- **Without trailing slash**: `metadata/automation`

All formats work identically. The tool automatically normalizes the URI and determines whether you're fetching a specific resource or browsing a category.

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
