# Remote Docker Example - HTTP Deployment

This example shows how to deploy TheHiveMCP as a remote HTTP service using Docker.

## Security requirements

TheHiveMCP serves plain HTTP and doesn't authenticate callers itself. Before exposing it beyond localhost:

- **Always place a TLS-terminating, authenticating reverse proxy** (nginx, Traefik, Caddy, and so on) in front of the MCP port. Never expose port 8082 directly
  to untrusted networks.
- **Clients must send their own TheHive credentials**: `Authorization: Bearer <key>` or `X-TheHive-Api-Key: <key>`, the latter winning when both are sent.
  Another `Authorization` scheme, such as your proxy's Basic credentials, is ignored and never forwarded to TheHive. Requests without credentials are rejected
  unless you explicitly set `ALLOW_ENV_CREDENTIAL_FALLBACK=true`, which makes those requests fall back to the server's `THEHIVE_API_KEY`; a request that sends
  its own key keeps it. Only do this for single-user deployments behind an authenticating proxy.
- **The `X-TheHive-Url` header is restricted** to the configured `THEHIVE_URL`. To allow additional TheHive instances (multi-tenant), list them explicitly in
  `THEHIVE_URL_ALLOWLIST` (comma-separated, exact scheme/host/port match). Any other destination is rejected before TheHive is contacted.
- **The server bounds slow and oversized requests itself**: 10 s to send the headers, 30 s for the whole request, 4 MiB per body. It does not cap how many
  connections or event streams a client opens, and anyone who reaches the port can open one without credentials: set connection limits on your proxy.

## Prerequisites

- Docker and Docker Compose installed
- TheHive 5.5+ instance accessible

## Setup

### 1. Configure environment

Create `.env` file:

```bash
# TheHive Configuration
THEHIVE_URL=https://your-thehive.com
THEHIVE_API_KEY=your-api-key
THEHIVE_ORGANISATION=your-org  # Optional, defaults to user's own organisation
# THEHIVE_REQUEST_TIMEOUT=2m  # Optional, maximum wait for TheHive to answer one request

# HTTP transport security (optional, defaults are the safe choice)
# THEHIVE_URL_ALLOWLIST=https://thehive-eu.example.com,https://thehive-us.example.com
# ALLOW_ENV_CREDENTIAL_FALLBACK=false
# AUTH_VALIDATION_CACHE_TTL=60s
```

### A fuller example

```bash
# .env file - Base configuration
THEHIVE_URL=https://thehive.example.com
THEHIVE_API_KEY=<thehive_api_key>
THEHIVE_ORGANISATION=<thehive_organisation>  # Optional, defaults to user's own organisation
# THEHIVE_REQUEST_TIMEOUT=2m  # Optional, maximum wait for TheHive to answer one request
PERMISSIONS_CONFIG=docs/reference/permissions-examples/analyst.yaml  # Optional, defaults to read-only
MCP_BIND_HOST=0.0.0.0  # Exposes the server to the network: requires a TLS-terminating, authenticating reverse proxy in front
MCP_PORT=8082
# HTTP transport security (defaults shown):
# THEHIVE_URL_ALLOWLIST=https://thehive-eu.example.com,https://thehive-us.example.com  # X-TheHive-Url targets (defaults to THEHIVE_URL only)
# ALLOW_ENV_CREDENTIAL_FALLBACK=false  # Set to true ONLY for single-user deployments where requests may omit credentials
# AUTH_VALIDATION_CACHE_TTL=60s
LOG_LEVEL=INFO
# Permissions options (choose one):
# PERMISSIONS_CONFIG=read_only                              # Default: safe read-only access
# PERMISSIONS_CONFIG=admin                                  # Full access (development/testing only)
# PERMISSIONS_CONFIG=docs/reference/permissions-examples/analyst.yaml # Custom permissions file

# Optional Cortex configuration:
# CORTEX_ID=local               # Default Cortex instance ID (defaults to 'local')
```

Every variable, flag and header is listed in [the configuration reference](../reference/configuration.md).

### 2. Deploy

```bash
# Start the service
docker compose -f deployment/examples/docker-compose.basic.yml up -d

# Check status
docker compose -f deployment/examples/docker-compose.basic.yml ps
```

## What the compose file starts

The [`docker-compose.basic.yml`](../../deployment/examples/docker-compose.basic.yml) provides:

- **HTTP server** on port 8082
- **Read-only permissions** by default
- **Environment or header configuration**
- **Health checks** and auto-restart

## Connect an MCP client

MCP clients connect to `http://your-server:8082/mcp` and can:

- Use environment variables for TheHive connection
- Override with HTTP headers per request
- Access all MCP tools (search, manage, execute, resources)

### Override settings per request

**HTTP Headers** (HTTP transport only):

```bash
curl -X POST http://localhost:8082/mcp \
  -H "Authorization: Bearer different-api-key" \
  -H "X-TheHive-Org: other-org" \
  -H "X-TheHive-Url: https://other-thehive.com" \
  -d '{"method":"initialize",...}'
```

The headers and their rules are in [the configuration reference](../reference/configuration.md#per-request-headers).

## Next steps

- For local development: [stdio Example](stdio-local.md)
- For LibreChat integration: [LibreChat Example](librechat.md)
- For custom permissions: [Configure permissions](configure-permissions.md)
