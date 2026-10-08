# Configure permissions

This guide shows how to start the server with a permission profile, how permissions apply in each deployment mode, and what to check when a call is refused. The
profile format is in [the permissions reference](../reference/permissions.md), and `PERMISSIONS_CONFIG` is defined in
[the configuration reference](../reference/configuration.md).

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
# Bundle a permissions config with the MCPB of your machine
make mcpb-local PERMISSIONS_CONFIG=docs/reference/permissions-examples/analyst.yaml
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

**"Analyzer/Responder … is not among the N … TheHive lists"**

- The rules name specific workers, and this one is not in the analyzer catalog or in the responders listed for that entity. Use an analyzer ID or name from
  `hive://metadata/automation/analyzers`, or a responder ID from `hive://metadata/automation/responders?entityType=<type>&entityId=<id>`: a responder must be
  run by its ID.
- N = 0 means TheHive listed nothing, which it does when it cannot reach Cortex.

**"not found or is not within the scope permitted"**

- The entity is excluded by the tool's configured `filters` (or does not exist)
- Check active permissions: `get-resource hive://config/permissions`

**Empty search results**

- Permission filters may be restricting results
- Check active permissions: `get-resource hive://config/permissions`

**"No permissions found in context"**

- System configuration error
- Check server logs for permission loading errors
