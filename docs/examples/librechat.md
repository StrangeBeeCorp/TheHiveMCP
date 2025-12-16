# LibreChat example - simple integration

This example shows how to run TheHiveMCP with **LibreChat** using Anthropic models for AI-powered TheHive operations.

> **⚠️ BETA WARNING**: Use only with test data and development TheHive instances.

## Overview

This setup provides a web-based chat interface where users can interact with TheHive through natural language using Claude models. The configuration includes:

- [`docker-compose.librechat.yml`](docker/docker-compose.librechat.yml) - Complete LibreChat stack with MongoDB, Meilisearch, and API
- [`librechat.yaml`](docker/librechat.yaml) - MCP server configuration with user variables
- [`Dockerfile.librechat`](docker/Dockerfile.librechat) - Custom LibreChat build with MCP support

## Prerequisites

- Docker and Docker Compose installed
- TheHive 5.x instance accessible
- Anthropic API key

## Setup

### 1. Configure environment

Create `.env` file with only the required variable:

```bash
# Anthropic Configuration (only required variable)
ANTHROPIC_API_KEY=sk-ant-your-anthropic-key-here
```

The Docker Compose file already includes all other necessary configuration with hardcoded values for development use.

### 2. Start services

```bash
# Start all services
docker-compose -f docs/examples/docker/docker-compose.librechat.yml up -d

# Check status
docker-compose -f docs/examples/docker/docker-compose.librechat.yml ps
```

### 3. Access LibreChat

1. **Open browser**: http://localhost:3080
2. **Create account** and login
3. **Select Claude model** from the model dropdown
4. **Configure TheHive connection**: You'll be prompted to enter:
   - TheHive URL (e.g., `https://your-thehive.com`)
   - API Key
   - Organisation name

## How it works

The [`librechat.yaml`](docker/librechat.yaml) configuration enables per-user TheHive credentials. It defines custom user variables that LibreChat will prompt for and pass as headers to TheHiveMCP.

When you start a conversation, LibreChat will prompt you for these values and send them as headers to TheHiveMCP for each request.

The [`docker-compose.librechat.yml`](docker/docker-compose.librechat.yml) includes:
- **TheHiveMCP server** with admin permissions and Anthropic fallback
- **LibreChat API** with Anthropic endpoint enabled
- **MongoDB** and **Meilisearch** for LibreChat's backend

## What to expect

Once configured, you can:

- **Search entities**: "Find critical alerts from last week"
- **Create cases**: "Create a new phishing investigation case"
- **Run analyzers**: "Analyze this IP with VirusTotal"
- **Get information**: "Show me the alert schema"

The AI assistant will use TheHiveMCP tools to interact with your TheHive instance and provide structured responses.

## Services

The stack includes:
- **LibreChat API** (port 3080) - Web interface
- **TheHiveMCP** (port 8082) - MCP server
- **MongoDB** (port 27017) - Database
- **Meilisearch** (port 7700) - Search engine

## Next steps

- For simpler deployment: [Remote Docker Example](remote-docker.md)
- For local development: [stdio Example](stdio-local.md)
- For custom permissions: [Permissions Guide](../permissions.md)
