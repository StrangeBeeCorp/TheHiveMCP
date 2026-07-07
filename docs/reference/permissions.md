# TheHiveMCP Permissions System

## Overview

The permissions system provides fine-grained access control over TheHive operations:

- **Tool access**: Control which MCP tools can be used
- **Data filtering**: Restrict what data can be accessed via queries
- **Automation control**: Manage which analyzers and responders can be executed

**Default**: Read-only access when no configuration is specified.

## Quick Start

```bash
# Uses default read-only permissions
./thehivemcp

# Specify custom permissions
./thehivemcp --permissions-config /path/to/permissions.yaml

# Or via environment variable
export PERMISSIONS_CONFIG=/path/to/permissions.yaml
./thehivemcp
```

## Configuration Format

```yaml
version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
      filters: # Optional: restrict data access
        _ne:
          _field: "status"
          _value: "Deleted"
    manage-entities:
      allowed: false
    execute-automation:
      allowed: true
    get-resource:
      allowed: true

  analyzers:
    mode: "allow_list" # or "block_list"
    allowed: ["VirusTotal_3_0", "Shodan_Host"]

  responders:
    mode: "block_list"
    blocked: ["DeleteCase_1_0", "PurgeAlert_1_0"]
```

## Tool Permissions

### Available tools

- `search-entities`: Search and query TheHive entities
- `manage-entities`: Create, update, delete entities
- `execute-automation`: Run analyzers and responders
- `get-resource`: Access documentation, schemas, and metadata

### Tool filters

Filters restrict which entities a tool can reach. They can be configured per tool:

```yaml
tools:
  search-entities:
    allowed: true
    filters:
      _gte:
        _field: "severity"
        _value: 2
  manage-entities:
    allowed: true
    filters:
      _eq:
        _field: "tags"
        _value: "soc-l1"
  execute-automation:
    allowed: true
    filters:
      _lte:
        _field: "tlp"
        _value: 2
```

Uses TheHive's native filter syntax where the operator (for example, `_gte`, `_lte`, `_eq`) is the key, with `_field` and `_value` as properties.

**How filters are enforced per tool:**

Filters are always evaluated by TheHive itself, using its native query language — the MCP server never reimplements filtering. For tools that act on a raw
entity ID, the server issues a _scoped existence check_ before doing anything: it asks TheHive for that specific entity with the configured filter appended as a
`filter` stage (e.g. `getCase {id} → filter {tlp ≤ 2}`). If TheHive returns the entity, it is in scope; if it returns nothing (filtered out, deleted, or not
visible to the caller's API key), the operation is denied. This reuses the exact same filter the deployment configures for search.

- `search-entities`: the filter is AND-merged directly into the search query, so results are scoped server-side. The same applies to additional-query expansion
  (fetching a case's tasks, observables, comments, etc.): the parent entity is scope-checked before its children are fetched.
- `manage-entities`: before any by-ID operation (update, delete, comment, promote, merge, apply-template, or creating a child entity inside a case/alert), every
  referenced entity is scope-checked. An entity outside the filter is reported as "not found or not within the scope" and nothing is mutated.
- `execute-automation`: the filter applies to the **entity the automation acts on**, not to the analyzer or responder. Before running, the target is
  scope-checked — the observable for `run-analyzer`, the entity (case/alert/task/observable) for `run-responder` and `get-action-status`. For `get-job-status`,
  the job's target observable is resolved server-side (`getJob → observable`) and scope-checked before the report is returned. (Analyzers and responders
  themselves are gated separately by the allow/block lists below, not by these filters.)

A filter that **is not configured** for a tool means "no restriction" — that tool behaves exactly as before (backward compatible). Only configured filters
constrain reach.

**Known limitations** (fail closed where applicable):

- _Creating top-level entities_ (alerts, cases, case templates, standalone pages) is not constrained by filters: filters scope reach to existing entities, and a
  brand-new top-level entity reaches none. An agent can therefore create an entity that its own filter then hides from it. Use `entity_permissions` to deny
  `create` if needed.
- _Case template targets of `apply-template`_ are org-level configuration, not row-scoped data; the template itself is not checked against the filter. The cases
  the template is applied to **are** checked.
- _Filter fields must exist on the entity types the tool touches._ If a filter references a field that an entity type does not have (for example a `tlp` filter
  checked against a procedure), the scope query fails and the operation is **denied** (fail closed), not silently allowed.

### Granular entity permissions (manage-entities only)

Control specific operations on each entity type:

```yaml
tools:
  manage-entities:
    allowed: true
    entity_permissions:
      alert:
        create: true
        update: true
        delete: false # Deny delete for analysts
        comment: true
        promote: true # Allow promoting alerts to cases
        merge: true # Allow merging alerts into cases
      case:
        create: true
        update: true
        delete: false
        comment: true
        promote: false # N/A for cases
        merge: true # Allow merging cases together
      task:
        create: true
        update: true
        delete: false
        comment: true
        promote: false # N/A for tasks
        merge: false # N/A for tasks
      observable:
        create: true
        update: true
        delete: false
        comment: true
        promote: false # N/A for observables
        merge: true # Allow deduplicating observables
      procedure:
        create: true
        update: true
        delete: false
        comment: false # N/A for procedures
        promote: false # N/A for procedures
        merge: false # N/A for procedures
```

**Behavior:**

- If no `entity_permissions` are configured: all operations allowed (backward compatibility)
- If `entity_permissions` are configured: only specified entity types/operations allowed
- Entity types: `alert`, `case`, `task`, `observable`, `procedure`
- Operations: `create`, `update`, `delete`, `comment`, `promote`, `merge`

## Automation Permissions

### Allow list mode

Only specified items are permitted:

```yaml
analyzers:
  mode: "allow_list"
  allowed: ["VirusTotal_3_0", "Shodan_Host", "*"] # "*" = all
```

### Block list mode

All items except those blocked:

```yaml
responders:
  mode: "block_list"
  blocked: ["DeleteCase_1_0"]
```

## Example Configurations

### Read-only (default)

```yaml
version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
    manage-entities:
      allowed: false
    execute-automation:
      allowed: false
    get-resource:
      allowed: true
  analyzers:
    mode: "allow_list"
    allowed: []
  responders:
    mode: "allow_list"
    allowed: []
```

See: [read-only.yaml](permissions-examples/read-only.yaml)

### Analyst

```yaml
version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
      # Restrict to TLP and PAP equal or below 2 (AMBER)
      filters:
        _and:
          - _lte:
              _field: "tlp"
              _value: 2
          - _lte:
              _field: "pap"
              _value: 2
    manage-entities:
      allowed: true
      entity_permissions:
        alert:
          create: true
          update: true
          delete: false # Analysts cannot delete alerts
          comment: true
          promote: true # Allow alert promotion
          merge: true # Allow merging alerts
        case:
          create: true
          update: true
          delete: false # Analysts cannot delete cases
          comment: true
          promote: false # N/A for cases
          merge: true # Allow case merging
        task:
          create: true
          update: true
          delete: false
          comment: true
          promote: false # N/A for tasks
          merge: false # N/A for tasks
        observable:
          create: true
          update: true
          delete: false
          comment: true
        procedure:
          create: true
          update: true
          delete: false
          comment: false # N/A for procedures
    execute-automation:
      allowed: true
    get-resource:
      allowed: true
  analyzers:
    mode: "allow_list"
    allowed: ["VirusTotal_3_0", "Shodan_Host", "MISP_2_0"]
  responders:
    mode: "block_list"
    blocked: ["DeleteCase_1_0", "PurgeAlert_1_0"]
```

See: [analyst.yaml](permissions-examples/analyst.yaml)

### Administrator

```yaml
version: "1.0"
permissions:
  tools:
    search-entities:
      allowed: true
    manage-entities:
      allowed: true
    execute-automation:
      allowed: true
    get-resource:
      allowed: true
  analyzers:
    mode: "allow_list"
    allowed: ["*"]
  responders:
    mode: "allow_list"
    allowed: ["*"]
```

See: [admin.yaml](permissions-examples/admin.yaml)

## Checking Active Permissions

Query current permissions via MCP resource:

```bash
get-resource hive://config/permissions
```

## Deployment Modes

Works uniformly across all modes:

**STDIO:**

```bash
./thehivemcp --transport stdio --permissions-config permissions.yaml
```

**HTTP:**

```bash
./thehivemcp --transport http --permissions-config permissions.yaml
```

**In-Process:**

```go
mcpServer := bootstrap.GetInprocessServer(creds, "/path/to/permissions.yaml")
```

**Docker:**

```bash
# Mount permissions config into container
docker run -d \
  -v /host/path/analyst.yaml:/app/permissions.yaml \
  -e PERMISSIONS_CONFIG=/app/permissions.yaml \
  -e THEHIVE_URL=https://thehive.example.com \
  -e THEHIVE_API_KEY=your-api-key \
  ghcr.io/strangebeecorp/thehivemcp/thehivemcp:latest
```

**MCPB (MCP bundle):**

When generating MCPB packages, permissions configs can be bundled directly:

```bash
# Bundle a permissions config with the MCPB
export PERMISSIONS_CONFIG=docs/reference/permissions-examples/analyst.yaml
./scripts/generate-mcpb.sh
```

The permissions file will be:

- Copied into the MCPB as `permissions.yaml`
- Set as the default value in the user configuration
- Users can override with their own path after installation

Alternatively, users can specify a permissions path when configuring the MCPB in their MCP client.

## Best Practices

1. **Start restrictive**: Begin with read-only, add permissions as needed
2. **Use specific IDs**: Prefer explicit analyzer/responder IDs over wildcards
3. **Test configurations**: Verify with `get-resource hive://config/permissions`
4. **Document changes**: Add comments to your YAML files

## Troubleshooting

**"Tool is not permitted"**

- Check your permissions file: `allowed: true` for the tool

**"Analyzer/Responder is not permitted"**

- Add to `allowed` list or remove from `blocked` list

**"not found or is not within the scope permitted"**

- The entity is excluded by the tool's configured `filters` (or does not exist)
- Check active permissions: `get-resource hive://config/permissions`

**Empty search results**

- Permission filters may be restricting results
- Check active permissions: `get-resource hive://config/permissions`

**"No permissions found in context"**

- System configuration error
- Check server logs for permission loading errors

## Security

- **Default Deny**: All operations denied unless explicitly allowed
- **No Runtime Changes**: Permissions loaded once at startup
- **Filter Enforcement**: Permission filters are merged into search queries and verified server-side before manage, expansion, and automation operations reach
  an entity (see "Tool filters" above for the exact guarantees and limitations)
- **Untrusted-data wrapping (deny-by-default)**: TheHive field values returned to the LLM are wrapped in `[UNTRUSTED_DATA]...[/UNTRUSTED_DATA]` boundary tags so
  the model can tell data from instructions. The policy is **deny-by-default**: every string value is wrapped _unless_ its field name is on a small trusted
  allowlist of structural identifiers, enums/control values, and dates (`_id`, `_type`, `status`, `dataType`, date fields, …). This means `customFields` values,
  attachment names, and any field added to the TheHive SDK in the future are wrapped automatically — there is no longer an allowlist of "untrusted" fields that
  has to be kept in sync. Embedded boundary markers are escaped so the boundary cannot be broken out of.
- **Logged Operations**: Permission denials logged for auditing

## TheHive Filter Syntax

Common operators for filters:

- `_eq`, `_ne`: Equals, not equals
- `_gt`, `_gte`, `_lt`, `_lte`: Comparisons
- `_in`: Value in list
- `_like`: String pattern matching
- `_and`, `_or`, `_not`: Logical operators

Example:

```yaml
filters:
  _and:
    - _gte:
        _field: "severity"
        _value: 2
    - _in:
        _field: "status"
        _value: ["Open", "InProgress"]
```
