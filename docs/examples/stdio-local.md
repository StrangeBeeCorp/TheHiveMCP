# stdio Example - Local MCP Host Integration

This example shows how to run TheHiveMCP in **stdio mode** for integration with local MCP hosts like GitHub Copilot, Claude Desktop, or custom MCP applications.

> **⚠️ BETA WARNING**: Use only with test data and development TheHive instances.

## Use Case

- **GitHub Copilot integration** via MCP
- **Claude Desktop** personal usage
- **Local development** and testing
- **Custom MCP applications** using stdio transport

## Prerequisites

- TheHive 5.x instance with API access
- TheHive API key and organization name
- MCP host that supports stdio transport

## Quick Start

### 1. Download Binary

```bash
# Download for your platform
curl -L -o thehivemcp https://github.com/StrangeBeeCorp/TheHiveMCP/releases/latest/download/thehivemcp-darwin-arm64
chmod +x thehivemcp
```

### 2. Configure Environment

Create `.env` file:

```bash
# .env - TheHive connection
THEHIVE_URL=https://your-thehive-instance.com
THEHIVE_API_KEY=your-api-key-here
THEHIVE_ORGANISATION=your-org-name

# Permissions (choose one)
PERMISSIONS_CONFIG=read_only    # Safe default (recommended)
# PERMISSIONS_CONFIG=admin      # Full access (development only)

# Logging
LOG_LEVEL=info
```

### 3. Test Connection

```bash
# Load environment
source .env

# Test TheHive connectivity
curl -k -H "Authorization: Bearer $THEHIVE_API_KEY" \
  "$THEHIVE_URL/api/v1/user/current"
```

## MCP Host Integration

### GitHub Copilot

Configure in your MCP settings:

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

Add to Claude Desktop configuration:

```json
{
  "mcpServers": {
    "TheHiveMCP": {
      "command": "/path/to/thehivemcp",
      "args": ["--transport", "stdio"],
      "env": {
        "THEHIVE_URL": "https://your-thehive-instance.com",
        "THEHIVE_API_KEY": "your-api-key-here",
        "THEHIVE_ORGANISATION": "your-org-name",
        "PERMISSIONS_CONFIG": "read_only",
        "LOG_LEVEL": "info"
      }
    }
  }
}
```

### Manual Testing

```bash
# Run stdio mode manually
source .env
./thehivemcp --transport stdio

# Send test MCP message (in another terminal)
echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"1.0.0","clientInfo":{"name":"test","version":"1.0"}},"id":1}' | ./thehivemcp --transport stdio
```

## Features

### ✅ Supported with Claude Desktop
- **🤖 Natural language queries** ("show high severity alerts from last week")
- **🛡️ User confirmation** for modifications (elicitation)
- **🔍 Entity search** and browsing
- **📊 Resource access** (schemas, metadata)

### ⚠️ Limited with Other Clients
- **🤖 Natural language queries** require OpenAI fallback
- **🛡️ No user confirmation** (operations proceed automatically)
- **🔍 Entity search** works with filter syntax only
- **📊 Resource access** fully supported

## Troubleshooting

### "Natural language queries not working"
```bash
# Add OpenAI fallback for non-Claude clients
export OPENAI_API_KEY=sk-your-openai-key
export OPENAI_BASE_URL=https://api.openai.com/v1
```

### "Permission denied"
```bash
# Check permissions config
echo $PERMISSIONS_CONFIG

# Use admin for full access (development only)
export PERMISSIONS_CONFIG=admin
```

### "Connection refused"
```bash
# Test TheHive connectivity
curl -k "$THEHIVE_URL/api/v1/status"

# Test API key
curl -k -H "Authorization: Bearer $THEHIVE_API_KEY" \
  "$THEHIVE_URL/api/v1/user/current"
```

## Security Notes

- **Use `read_only` permissions** for production/personal use
- **Only use `admin` permissions** for development and testing
- **Keep API keys secure** - never commit to version control
- **Test with non-production data** in beta phase

## Next Steps

- For remote/team deployments: [Remote Docker Example](remote-docker.md)
- For LibreChat integration: [LibreChat Example](librechat.md)
- For custom permissions: [Permissions Guide](../permissions.md)
