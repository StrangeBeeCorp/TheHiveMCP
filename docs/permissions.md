# TheHiveMCP Permissions System

## Overview

The permissions system provides fine-grained access control over TheHive operations:

- **Tool Access**: Control which MCP tools can be used
- **Data Filtering**: Restrict what data can be accessed via queries
- **Automation Control**: Manage which analyzers and responders can be executed

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
      filters:  # Optional: restrict data access
        _field: "status"
        _operator: "_ne"
        _value: "Deleted"
    manage-entities:
      allowed: false
    execute-automation:
      allowed: true
      analyzer_restrictions:  # Optional: tool-specific restrictions
        mode: "allow_list"
        allowed: ["VirusTotal_3_0", "Shodan_Host"]
      responder_restrictions:
        mode: "block_list"
        blocked: ["DeleteCase_1_0"]
    get-resource:
      allowed: true

  analyzers:
    mode: "allow_list"  # or "block_list"
    allowed: ["VirusTotal_3_0", "Shodan_Host"]

  responders:
    mode: "block_list"
    blocked: ["DeleteCase_1_0", "PurgeAlert_1_0"]
```

## Tool Permissions

### Available Tools

- `search-entities`: Search and query TheHive entities
- `manage-entities`: Create, update, delete entities
- `execute-automation`: Run analyzers and responders
- `get-resource`: Access documentation, schemas, and metadata

### Tool Filters (search-entities only)

Filters are automatically merged with user queries using AND logic:

```yaml
tools:
  search-entities:
    allowed: true
    filters:
      _field: "severity"
      _operator: "_gte"
      _value: 2
```

Uses TheHive's native filter syntax.

### Granular Entity Permissions (manage-entities only)

Control specific operations on each entity type:

```yaml
tools:
  manage-entities:
    allowed: true
    entity_permissions:
      alert:
        create: true
        update: true
        delete: false  # Deny delete for analysts
        comment: true
      case:
        create: true
        update: true
        delete: false
        comment: true
      task:
        create: true
        update: true
        delete: false
        comment: true
      observable:
        create: true
        update: true
        delete: false
        comment: true
```

**Behavior:**
- If no `entity_permissions` configured: all operations allowed (backward compatibility)
- If `entity_permissions` configured: only specified entity types/operations allowed
- Entity types: `alert`, `case`, `task`, `observable`
- Operations: `create`, `update`, `delete`, `comment`

## Automation Permissions

### Allow List Mode
Only specified items are permitted:

```yaml
analyzers:
  mode: "allow_list"
  allowed: ["VirusTotal_3_0", "Shodan_Host", "*"]  # "*" = all
```

### Block List Mode
All items except those blocked:

```yaml
responders:
  mode: "block_list"
  blocked: ["DeleteCase_1_0"]
```

### Tool-Specific Restrictions
Override global settings per tool:

```yaml
tools:
  execute-automation:
    analyzer_restrictions:
      mode: "allow_list"
      allowed: ["VirusTotal_3_0"]  # Overrides global
```

## Example Configurations

### Read-Only (Default)

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

See: [read-only.yaml](examples/permissions/read-only.yaml)

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
          delete: false  # Analysts cannot delete alerts
          comment: true
        case:
          create: true
          update: true
          delete: false  # Analysts cannot delete cases
          comment: true
        task:
          create: true
          update: true
          delete: false
          comment: true
        observable:
          create: true
          update: true
          delete: false
          comment: true
    execute-automation:
      allowed: true
      analyzer_restrictions:
        mode: "allow_list"
        allowed: ["VirusTotal_3_0", "Shodan_Host", "MISP_2_0"]
      responder_restrictions:
        mode: "block_list"
        blocked: ["DeleteCase_1_0"]
    get-resource:
      allowed: true
  analyzers:
    mode: "allow_list"
    allowed: ["VirusTotal_3_0", "Shodan_Host", "MISP_2_0"]
  responders:
    mode: "block_list"
    blocked: ["DeleteCase_1_0"]
```

See: [analyst.yaml](examples/permissions/analyst.yaml)

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

See: [admin.yaml](examples/permissions/admin.yaml)

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
  strangebee/thehive-mcp:latest
```

**MCPB (MCP Bundle):**

When generating MCPB packages, permissions configs can be bundled directly:

```bash
# Bundle a permissions config with the MCPB
export PERMISSIONS_CONFIG=docs/examples/permissions/analyst.yaml
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

**Empty search results**
- Permission filters may be restricting results
- Check active permissions: `get-resource hive://config/permissions`

**"No permissions found in context"**
- System configuration error
- Check server logs for permission loading errors

## Security

- **Default Deny**: All operations denied unless explicitly allowed
- **No Runtime Changes**: Permissions loaded once at startup
- **Filter Merging**: Permission filters merged with user queries at query time
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
    - _field: "severity"
      _operator: "_gte"
      _value: 2
    - _field: "status"
      _operator: "_in"
      _value: ["Open", "InProgress"]
```
