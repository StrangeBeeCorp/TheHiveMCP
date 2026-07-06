# Remote docker example - HTTP deployment

This example shows how to deploy TheHiveMCP as a remote HTTP service using Docker.

## Security requirements

TheHiveMCP serves plain HTTP and does not authenticate callers itself. Before exposing it beyond localhost:

- **Always place a TLS-terminating, authenticating reverse proxy** (nginx, Traefik, Caddy, ...) in front of the MCP port. Never expose port 8082 directly to
  untrusted networks.
- **Clients must send their own TheHive credentials** (`Authorization` or `X-TheHive-Api-Key` header). Requests without credentials are rejected unless you
  explicitly set `ALLOW_ENV_CREDENTIAL_FALLBACK=true`, which makes every request fall back to the server's `THEHIVE_API_KEY` — only do this for single-user
  deployments behind an authenticating proxy.
- **The `X-TheHive-Url` header is restricted** to the configured `THEHIVE_URL`. To allow additional TheHive instances (multi-tenant), list them explicitly in
  `THEHIVE_URL_ALLOWLIST` (comma-separated, exact scheme/host/port match). Any other destination is rejected before TheHive is contacted.

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

# HTTP transport security (optional, defaults are the safe choice)
# THEHIVE_URL_ALLOWLIST=https://thehive-eu.example.com,https://thehive-us.example.com
# ALLOW_ENV_CREDENTIAL_FALLBACK=false
# AUTH_VALIDATION_CACHE_TTL=60s
```

### 2. Deploy

```bash
# Start the service
docker compose -f docs/examples/docker/docker-compose.basic.yml up -d

# Check status
docker compose -f docs/examples/docker/docker-compose.basic.yml ps
```

## How it works

The [`docker-compose.basic.yml`](docker/docker-compose.basic.yml) provides:

- **HTTP server** on port 8082
- **Read-only permissions** by default
- **Environment or header configuration**
- **Health checks** and auto-restart

## Usage

MCP clients connect to `http://your-server:8082/mcp` and can:

- Use environment variables for TheHive connection
- Override with HTTP headers per request
- Access all MCP tools (search, manage, execute, resources)

## Next steps

- For local development: [stdio Example](stdio-local.md)
- For LibreChat integration: [LibreChat Example](librechat.md)
- For custom permissions: [Permissions Guide](../permissions.md)
