# Remote Docker Example - HTTP Deployment

This example shows how to deploy TheHiveMCP as a remote HTTP service using Docker.

> **⚠️ BETA WARNING**: Use only with test data and development TheHive instances.

## Prerequisites

- Docker and Docker Compose installed
- TheHive 5.x instance accessible

## Setup

### 1. Configure Environment

Create `.env` file:

```bash
# TheHive Configuration
THEHIVE_URL=https://your-thehive.com
THEHIVE_API_KEY=your-api-key
THEHIVE_ORGANISATION=your-org
```

### 2. Deploy

```bash
# Start the service
docker-compose -f docs/examples/docker/docker-compose.basic.yml up -d

# Check status
docker-compose -f docs/examples/docker/docker-compose.basic.yml ps
```

## How It Works

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

## Next Steps

- For local development: [stdio Example](stdio-local.md)
- For LibreChat integration: [LibreChat Example](librechat.md)
- For custom permissions: [Permissions Guide](../permissions.md)
