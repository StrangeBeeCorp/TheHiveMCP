# stdio Example - Local MCP Host Integration

This example shows how to run TheHiveMCP in stdio mode for integration with local MCP hosts like GitHub Copilot and Claude Desktop.

> **⚠️ BETA WARNING**: Use only with test data and development TheHive instances.

## Prerequisites

- TheHive 5.x instance with API access
- TheHive API key and organisation name
- MCP host that supports stdio transport

## Setup

### 1. Download binary

```bash
# Download for your platform from releases
curl -L -o thehivemcp https://github.com/StrangeBeeCorp/TheHiveMCP/releases/latest/download/thehivemcp-darwin-arm64
chmod +x thehivemcp
```

### 2. Configure environment

Using the [`.env.template`](../../.env.template):

```bash
# Copy template and configure
cp .env.template .env

# Edit with your TheHive details
THEHIVE_URL=https://your-thehive-instance.com
THEHIVE_API_KEY=your-api-key-here
THEHIVE_ORGANISATION=your-org-name
PERMISSIONS_CONFIG=read_only
```

## MCP host integration

### GitHub Copilot

Add to your MCP settings:

```json
{
  "mcpServers": {
    "thehive": {
      "command": "/path/to/thehivemcp",
      "args": ["--transport", "stdio"],
      "env": {
        "THEHIVE_URL": "https://your-thehive-instance.com",
        "THEHIVE_API_KEY": "your-api-key-here",
        "THEHIVE_ORGANISATION": "your-org-name",
        "PERMISSIONS_CONFIG": "read_only"
      }
    }
  }
}
```

### Claude Desktop

Claude Desktop does not support sampling. So you need to add openAI credentials.

Similar configuration, or use the MCPB package for easier setup.

## What to expect

Once configured, your MCP host can:
- **Search entities**: "Find critical alerts from last week"
- **Access resources**: Browse TheHive schemas and documentation
- **Create/modify**: Depends on permissions configuration

## Next steps

- For team deployment: [Remote Docker Example](remote-docker.md)
- For LibreChat integration: [LibreChat Example](librechat.md)
- For custom permissions: [Permissions Guide](../permissions.md)
