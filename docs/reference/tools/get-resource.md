# get-resource

Access TheHive resources for documentation, schemas, and metadata.

## Overview

Read-only: this tool never modifies TheHive. See [Tool annotations](annotations.md).

The `get-resource` tool is the entry point for exploring TheHive's capabilities. It provides hierarchical access to documentation, schemas, and metadata through
a URI-based resource system.

## Resource Structure

Resources are organized hierarchically:

- `hive://catalog` - Directory of all categories
- `hive://config/*` - Session and system info
- `hive://schema/*` - Entity field definitions
- `hive://metadata/*` - Available options and choices
- `hive://docs/*` - Documentation and guides

## Parameters

| Parameter | Type   | Required | Description                                                                                                            |
| --------- | ------ | -------- | ---------------------------------------------------------------------------------------------------------------------- |
| `uri`     | string | No       | Resource URI to query (for example, 'hive://schema/alert', 'hive://metadata/automation'). Omit to list all categories. |

## Usage

The tool automatically determines whether you're requesting a specific resource or browsing a category based on the URI provided.

**URI flexibility:**

- URIs work with or without the `hive://` prefix (for example, `"schema"` or `"hive://schema"`)
- Trailing slashes are handled automatically (for example, `"hive://schema/"` or `"hive://schema"`)

**Behavior:**

- If the URI points to a specific resource, the resource content is returned
- If the URI is a path, all resources and subcategories under that path are returned
- If the URI doesn't exist, an error is returned

### Discovery mode

Call without parameters to list all available categories:

```text
get-resource()
```

### Browse mode

Provide a URI to browse resources and subcategories at that path:

```text
get-resource(uri="hive://schema")
get-resource(uri="hive://metadata")
get-resource(uri="hive://metadata/automation")
get-resource(uri="hive://docs/entities")
```

### Fetch mode

Provide a URI to get a specific resource:

```text
get-resource(uri="hive://schema/alert")
get-resource(uri="hive://docs/entities/case")
get-resource(uri="hive://metadata/automation/analyzers")
```

## Examples

### Discovery and browsing

- **List all categories**: `get-resource()`
- **List schemas**: `get-resource(uri="hive://schema")`
- **Browse automation metadata**: `get-resource(uri="hive://metadata/automation")`
- **Browse entity docs**: `get-resource(uri="hive://docs/entities")`

### Specific resource fetching

- **Get alert output schema**: `get-resource(uri="hive://schema/alert")`
- **Get alert create schema**: `get-resource(uri="hive://schema/alert/create")`
- **Get alert update schema**: `get-resource(uri="hive://schema/alert/update")`
- **Get case documentation**: `get-resource(uri="hive://docs/entities/case")`
- **Get available analyzers**: `get-resource(uri="hive://metadata/automation/analyzers")`
- **Get analyzers for one observable type**: `get-resource(uri="hive://metadata/automation/analyzers?dataType=hash")`
- **Get available responders**: `get-resource(uri="hive://metadata/automation/responders?entityType=case&entityId=~123456")`

### Automation catalogs

Both automation catalogs return a page: `kind`, `total` (how many the caller may use), `returned`, `offset`, `truncated`, `blockedByPolicy`, and the `workers`
themselves. Check `truncated` before concluding a tool is unavailable, and `blockedByPolicy` before concluding the deployment has none: an empty catalog is
usually the permissions allow-list at work, not a missing Cortex. The shipped read-only default blocks every analyzer, so `total: 0` with a non-zero
`blockedByPolicy` means "your configuration hides these", not "they do not exist".

Unknown query parameters are rejected rather than ignored, so a misspelling (`?datatype=hash`) or a parameter borrowed from the other catalog
(`?entityType=observable` on the analyzers resource) fails loudly instead of returning the whole unfiltered catalog as if it were a filtered answer.

| Parameter    | Applies to            | Description                                                                                           |
| ------------ | --------------------- | ----------------------------------------------------------------------------------------------------- |
| `dataType`   | analyzers             | Only analyzers accepting that observable type (`hash`, `ip`, `domain`, `url`, …). Resolved by Cortex. |
| `entityType` | responders            | Required. Entity kind the responder acts on (`case`, `alert`, `task`, `observable`).                  |
| `entityId`   | responders            | Required. Identifier of that entity.                                                                  |
| `offset`     | analyzers, responders | Index of the first entry to return. Defaults to `0`.                                                  |
| `limit`      | analyzers, responders | Page size. Defaults to `50`, capped at `500`.                                                         |

`dataType` is the cheap way to answer "what can I run on this observable?" — Cortex filters server-side instead of the catalog being fetched and sifted locally.

Responders are listed per entity because that path is the one applying TLP and PAP limits: a responder missing from it is not permitted on that entity, even if
it exists in Cortex.

## Schema Organisation

Entity schemas are organized into three variants:

- **Output schemas** (`hive://schema/{entity}`): Fields returned from TheHive API when querying entities
- **Create schemas** (`hive://schema/{entity}/create`): Required and optional fields for creating new entities
- **Update schemas** (`hive://schema/{entity}/update`): Partial fields available for updating existing entities

Available entities: `alert`, `case`, `task`, `observable`, `procedure`, `pattern`, `case-template`, `page`, `job`, `action`

`job` (Cortex analyzer runs) and `action` (Cortex responder runs) are output-only — they are produced by [`execute-automation`](execute-automation.md), not
created directly, so they have no `/create` or `/update` variant.

Example:

- `hive://schema/task` - Output schema for tasks (what you get from queries)
- `hive://schema/task/create` - Input schema for creating tasks
- `hive://schema/task/update` - Partial input schema for updating tasks

This organisation makes it clear which fields are required for creation vs available for updates.

## Related resources and tools

`get-resource` supplies the descriptive data the other tools consume:

- `hive://schema/{entity}` and its `/create` and `/update` variants — the field definitions used by [`search-entities`](search-entities.md) (filterable and
  returned fields) and [`manage-entities`](manage-entities.md) (creatable and updatable fields).
- `hive://metadata/automation/analyzers` and `hive://metadata/automation/responders` — the live catalog of automation available to
  [`execute-automation`](execute-automation.md).
- `hive://docs/overview/filter-dsl` — the filter grammar accepted by [`search-entities`](search-entities.md).
