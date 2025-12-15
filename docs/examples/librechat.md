# LibreChat Example - Complete Integration Setup

This example shows how to integrate TheHiveMCP with **LibreChat** for a complete AI assistant experience with TheHive security operations.

> **⚠️ BETA WARNING**: Use only with test data and development TheHive instances.

## Use Case

- **Complete AI assistant** with TheHive integration
- **Web-based interface** for security analysts
- **Multi-user support** with per-user TheHive configuration
- **Chat-based security operations** workflow

## Prerequisites

- Docker and Docker Compose installed
- TheHive 5.x instance accessible
- OpenAI API key (or other LLM provider)

## Quick Start

### 1. Clone Configuration Files

The provided configuration includes:
- `docker-compose.librechat.yaml` - Complete LibreChat stack with MongoDB, Meilisearch, and API
- `librechat.yaml` - MCP server configuration with user variables
- `.env.example` - Environment template

### 2. Setup Environment

```bash
# Copy environment template
cp docs/examples/docker/.env.example .env

# Edit with your configuration
nano .env
```

**Required environment variables:**
```bash
# OpenAI Configuration (required for LibreChat)
OPENAI_API_KEY=sk-your-openai-key-here

# MongoDB (used by LibreChat)
MONGO_URI=mongodb://mongodb:27017/librechat

# Meilisearch (used by LibreChat)
MEILI_MASTER_KEY=your-meili-master-key

# JWT Secret (for LibreChat sessions)
JWT_SECRET=your-jwt-secret-here
JWT_REFRESH_SECRET=your-jwt-refresh-secret

# Default Admin (optional)
UID=1000
GID=1000
```

### 3. Deploy Stack

```bash
# Start all services
docker-compose -f docs/examples/docker/docker-compose.librechat.yaml up -d

# Check status
docker-compose -f docs/examples/docker/docker-compose.librechat.yaml ps

# View logs
docker-compose -f docs/examples/docker/docker-compose.librechat.yaml logs -f
```

### 4. Access LibreChat

1. **Open browser**: http://localhost:3080
2. **Create account** or login
3. **Configure TheHive**: In chat, you'll be prompted for:
   - TheHive URL
   - API Key
   - Organisation name

## Architecture Overview

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   LibreChat     │───▶│   TheHiveMCP     │───▶│   TheHive API   │
│   (Port 3080)   │    │   (Port 8082)    │    │   (Your Hive)   │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                        │
         ▼                        ▼
┌─────────────────┐    ┌──────────────────┐
│    MongoDB      │    │   Meilisearch    │
│   (Port 27017)  │    │   (Port 7700)    │
└─────────────────┘    └──────────────────┘
```

## LibreChat Configuration

The `librechat.yaml` configuration enables:

### MCP Server Integration

```yaml
mcpServers:
  TheHiveMCP:
    type: streamable-http
    url: http://thehive-mcp:8082/mcp
    customUserVars:
      THEHIVE_URL:
        title: "TheHive URL"
        description: "Your TheHive instance URL (e.g. https://thehive.company.com)"
      THEHIVE_API_KEY:
        title: "TheHive API Key"
        description: "Your TheHive API key for authentication"
      THEHIVE_ORG:
        title: "Organisation"
        description: "Your TheHive organisation name"
    headers:
      X-TheHive-Url: "{{THEHIVE_URL}}"
      X-TheHive-Api-Key: "{{THEHIVE_API_KEY}}"
      X-TheHive-Org: "{{THEHIVE_ORG}}"
```

### User Experience

When users start a conversation, they'll see prompts for:
- **TheHive URL**: `https://your-thehive.com`
- **API Key**: Personal or shared API key
- **Organisation**: TheHive org name

These values are stored per-user and sent as headers to TheHiveMCP.

## Usage Examples

### Search for Cases

```
User: "Find all critical cases from last week"

AI: I'll search for critical cases from the past week. Let me query TheHive for you.

[Uses search-entities tool with natural language processing]

Found 5 critical cases from last week:
1. **Ransomware Investigation** - Case #2024-001
   - Status: InProgress
   - Severity: Critical
   - Created: 2024-01-15

2. **Data Breach Response** - Case #2024-002
   - Status: Open
   - Severity: Critical
   - Created: 2024-01-17
...
```

### Create New Case

```
User: "Create a new case for suspicious email attachment analysis"

AI: I'll create a new case for email attachment analysis.

[Uses manage-entities tool to create case with appropriate template]

✅ Created new case: **Suspicious Email Attachment Analysis**
- Case ID: #2024-015
- Status: Open
- Template: Email Analysis Template
- Assigned to: Current user

Would you like me to create any initial tasks or observables?
```

### Execute Analyzers

```
User: "Run VirusTotal analysis on this hash: d41d8cd98f00b204e9800998ecf8427e"

AI: I'll run VirusTotal analysis on that MD5 hash.

[Uses execute-automation tool to run analyzer]

🔍 VirusTotal Analysis Results:
- Hash: d41d8cd98f00b204e9800998ecf8427e
- Detection: 0/70 engines (Clean)
- File type: Empty file
- Analysis complete in 2.3s
```

## Multi-User Setup

### Team Configuration

Each user configures their own TheHive credentials:

```yaml
# User A Configuration
THEHIVE_URL: https://team-a.thehive.com
THEHIVE_API_KEY: team-a-api-key
THEHIVE_ORG: TeamA

# User B Configuration
THEHIVE_URL: https://team-b.thehive.com
THEHIVE_API_KEY: team-b-api-key
THEHIVE_ORG: TeamB
```

### Shared Instance

For shared TheHive with role-based access:

```yaml
# Shared Configuration
THEHIVE_URL: https://shared.thehive.com
THEHIVE_ORG: SharedOrg

# Each user uses their own API key
THEHIVE_API_KEY: user-specific-api-key
```

## Features & Capabilities

### ✅ Full MCP Integration
- **🔍 Natural language search** - "Find phishing cases"
- **📝 Entity management** - Create cases, tasks, observables
- **🤖 Analyzer execution** - Run automated analysis
- **📊 Resource access** - View schemas, facts, rules
- **👤 User confirmations** - LibreChat supports elicitation
- **🎯 Intelligent sampling** - AI-powered query processing

### ✅ LibreChat Features
- **💬 Chat interface** - Natural conversation with AI
- **👥 Multi-user support** - Individual user configurations
- **🔐 Authentication** - Secure login system
- **📱 Responsive design** - Works on desktop and mobile
- **🔍 Search history** - Find previous conversations
- **📂 Conversation organization** - Folders and tags

### ⚠️ Current Limitations
- **🔧 OpenAI dependency** - Requires LLM API key for LibreChat
- **🐳 Docker complexity** - Multiple services to manage
- **💾 Storage requirements** - MongoDB and Meilisearch data

## Production Deployment

### Resource Requirements

```yaml
services:
  librechat-api:
    deploy:
      resources:
        limits:
          memory: 2G
          cpus: "1.0"

  thehive-mcp:
    deploy:
      resources:
        limits:
          memory: 512M
          cpus: "0.5"

  mongodb:
    deploy:
      resources:
        limits:
          memory: 1G
          cpus: "0.5"
```

### Backup Strategy

```bash
# Backup MongoDB data
docker-compose exec mongodb mongodump --out /backup/$(date +%Y%m%d)

# Backup Meilisearch data
docker-compose exec meilisearch curl -X POST 'http://localhost:7700/dumps'
```

### SSL/TLS Configuration

Add nginx reverse proxy with SSL:

```nginx
server {
    listen 443 ssl;
    server_name librechat.your-domain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:3080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

## Troubleshooting

### LibreChat Issues

```bash
# Check LibreChat logs
docker-compose -f docker-compose.librechat.yaml logs librechat-api

# Reset user configuration
# Users can clear their MCP variables in LibreChat settings

# Database issues
docker-compose -f docker-compose.librechat.yaml restart mongodb
```

### MCP Connection Issues

```bash
# Test MCP endpoint
curl -X POST http://localhost:8082/mcp \
  -H "Content-Type: application/json" \
  -H "X-TheHive-Url: https://your-thehive.com" \
  -H "X-TheHive-Api-Key: your-api-key" \
  -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"1.0.0","clientInfo":{"name":"test","version":"1.0"}},"id":1}'

# Check MCP server logs
docker-compose -f docker-compose.librechat.yaml logs thehive-mcp
```

### Performance Issues

```bash
# Monitor resource usage
docker stats

# Scale services if needed
docker-compose -f docker-compose.librechat.yaml up -d --scale librechat-api=2
```

## Security Considerations

### Network Security

```yaml
# Restrict external access
services:
  mongodb:
    ports: []  # Remove external port exposure

  meilisearch:
    ports: []  # Remove external port exposure

  thehive-mcp:
    ports: []  # Only accessible via LibreChat
```

### User Authentication

```bash
# Enable registration restrictions
ALLOW_REGISTRATION=false

# Configure social login (optional)
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
```

### API Key Management

- Store API keys securely in environment variables
- Use per-user API keys when possible
- Implement key rotation procedures
- Monitor API usage for anomalies

## Next Steps

- For simpler deployment: [Remote Docker Example](remote-docker.md)
- For local development: [stdio Example](stdio-local.md)
- For custom permissions: [Permissions Guide](../permissions.md)
- LibreChat documentation: https://docs.librechat.ai/
