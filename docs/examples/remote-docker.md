# Remote Docker Example - Scalable HTTP Deployment

This example shows how to deploy TheHiveMCP as a **remote HTTP service** using Docker, with configuration via HTTP headers for multi-tenant scenarios.

> **⚠️ BETA WARNING**: Use only with test data and development TheHive instances.

## Use Case

- **Team deployments** with shared MCP server
- **Multi-tenant scenarios** with per-request configuration
- **Web application integration** via HTTP transport
- **Scalable production** deployments

## Prerequisites

- Docker and Docker Compose installed
- Remote server or cloud instance
- TheHive 5.x instances accessible from deployment location

## Quick Start

### 1. Create Docker Compose File

Create `docker-compose.yml`:

```yaml
services:
  thehive-mcp:
    image: ghcr.io/strangebee/thehivemcp/thehivemcp:latest
    restart: unless-stopped
    ports:
      - "8082:8082"
    environment:
      # MCP Server Configuration
      - MCP_BIND_HOST=0.0.0.0
      - MCP_PORT=8082
      - MCP_ENDPOINT_PATH=/mcp

      # Default Permissions (no TheHive config - using headers)
      - PERMISSIONS_CONFIG=read_only

      # Logging
      - LOG_LEVEL=info

    healthcheck:
      test: ["CMD", "wget", "--quiet", "--spider", "http://localhost:8082/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

### 2. Deploy

```bash
# Start the service
docker-compose up -d

# Check status
docker-compose ps
docker-compose logs -f thehive-mcp

# Test health endpoint
curl http://localhost:8082/health
```

## Client Configuration

### Per-Request Configuration via Headers

The server uses **HTTP headers** for TheHive connection details, allowing multiple teams/tenants to use the same server with different TheHive instances.

#### MCP Client Configuration Example

```javascript
// Configure your MCP client to send headers
const mcpClient = new MCPClient({
  url: 'http://your-server:8082/mcp',
  headers: {
    'X-TheHive-Url': 'https://team-a-thehive.com',
    'X-TheHive-Api-Key': 'team-a-api-key',
    'X-TheHive-Org': 'team-a-org'
  }
});
```

#### LibreChat Configuration

```yaml
# librechat.yaml
mcpServers:
  TheHiveMCP:
    type: streamable-http
    url: http://your-server:8082/mcp
    customUserVars:
      THEHIVE_URL:
        title: "TheHive URL"
      THEHIVE_API_KEY:
        title: "API Key"
      THEHIVE_ORG:
        title: "Organisation"
    headers:
      X-TheHive-Url: "{{THEHIVE_URL}}"
      X-TheHive-Api-Key: "{{THEHIVE_API_KEY}}"
      X-TheHive-Org: "{{THEHIVE_ORG}}"
```

#### cURL Testing

```bash
# Test MCP endpoint with headers
curl -X POST http://your-server:8082/mcp \
  -H "Content-Type: application/json" \
  -H "X-TheHive-Url: https://your-thehive.com" \
  -H "X-TheHive-Api-Key: your-api-key" \
  -H "X-TheHive-Org: your-org" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialize",
    "params": {
      "protocolVersion": "1.0.0",
      "clientInfo": {"name": "test", "version": "1.0"}
    },
    "id": 1
  }'
```

## Production Deployment

### Cloud Deployment

#### AWS ECS / Fargate

```bash
# Create task definition with the Docker image
# Set up load balancer pointing to port 8082
# Configure health checks on /health endpoint
```

#### Google Cloud Run

```bash
gcloud run deploy thehive-mcp \
  --image ghcr.io/strangebee/thehivemcp/thehivemcp:latest \
  --port 8082 \
  --set-env-vars MCP_BIND_HOST=0.0.0.0,PERMISSIONS_CONFIG=read_only \
  --allow-unauthenticated
```

#### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thehive-mcp
spec:
  replicas: 2
  selector:
    matchLabels:
      app: thehive-mcp
  template:
    spec:
      containers:
      - name: thehive-mcp
        image: ghcr.io/strangebee/thehivemcp/thehivemcp:latest
        ports:
        - containerPort: 8082
        env:
        - name: MCP_BIND_HOST
          value: "0.0.0.0"
        - name: PERMISSIONS_CONFIG
          value: "read_only"
        readinessProbe:
          httpGet:
            path: /health
            port: 8082
---
apiVersion: v1
kind: Service
metadata:
  name: thehive-mcp-service
spec:
  selector:
    app: thehive-mcp
  ports:
  - port: 80
    targetPort: 8082
```

### Reverse Proxy (Optional)

nginx configuration for SSL termination:

```nginx
server {
    listen 443 ssl;
    server_name thehive-mcp.your-domain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8082;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Preserve MCP headers
        proxy_pass_header X-TheHive-Url;
        proxy_pass_header X-TheHive-Api-Key;
        proxy_pass_header X-TheHive-Org;
    }
}
```

## Features & Limitations

### ✅ Supported Features
- **🌐 HTTP transport** for web integrations
- **🏢 Multi-tenant** via header-based configuration
- **📊 All MCP tools** (search, manage, execute, get-resource)
- **⚖️ Flexible permissions** (read_only default)

### ⚠️ Current Limitations
- **🤖 Limited natural language** support (most HTTP clients don't support sampling)
- **🛡️ No user confirmation** (most HTTP clients don't support elicitation)
- **🔧 OpenAI fallback required** for natural language queries with most clients

### Workarounds

Add OpenAI fallback for natural language support:

```yaml
services:
  thehive-mcp:
    # ... existing config
    environment:
      # ... existing env vars
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_BASE_URL=https://api.openai.com/v1
```

## Security

### Network Security

```bash
# Configure firewall (UFW example)
sudo ufw allow 8082/tcp
sudo ufw enable
```

### Resource Limits

```yaml
services:
  thehive-mcp:
    # ... existing config
    deploy:
      resources:
        limits:
          memory: 512M
          cpus: "0.5"
        reservations:
          memory: 256M
          cpus: "0.25"
```

## Monitoring

### Health Checks

```bash
# Basic health check
curl http://localhost:8082/health

# MCP initialization test
curl -X POST http://localhost:8082/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"1.0.0","clientInfo":{"name":"test","version":"1.0"}},"id":1}'
```

### Logging

```bash
# View logs
docker-compose logs -f thehive-mcp

# Increase log level for debugging
docker-compose exec thehive-mcp sh -c 'export LOG_LEVEL=debug'
```

## Troubleshooting

### "Connection refused"
```bash
# Check if service is running
docker-compose ps

# Check logs
docker-compose logs thehive-mcp

# Test health endpoint
curl http://localhost:8082/health
```

### "Header configuration not working"
```bash
# Verify headers are being sent
curl -v -X POST http://localhost:8082/mcp \
  -H "X-TheHive-Url: https://your-thehive.com" \
  # ... other headers

# Check server logs for header processing
docker-compose logs thehive-mcp | grep -i header
```

### "Natural language queries failing"
```bash
# Add OpenAI fallback
echo "OPENAI_API_KEY=sk-your-key" >> .env
docker-compose restart
```

## Next Steps

- For local development: [stdio Example](stdio-local.md)
- For LibreChat integration: [LibreChat Example](librechat.md)
- For custom permissions: [Permissions Guide](../permissions.md)
