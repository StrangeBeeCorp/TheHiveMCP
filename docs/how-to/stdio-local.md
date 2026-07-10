# stdio Example - Local MCP Host Integration

This example shows how to run TheHiveMCP in stdio mode for integration with local MCP hosts like GitHub Copilot and Claude Desktop.

## Prerequisites

- TheHive 5.5+ instance with API access
- TheHive API key (and optionally an organisation name)
- MCP host that supports stdio transport

## Setup

### 1. Download binary

Pick the artifact matching your OS and architecture from the [releases page](https://github.com/StrangeBeeCorp/TheHiveMCP/releases)—binaries are published for
`darwin-amd64`, `darwin-arm64`, `linux-amd64`, `linux-arm64`, `windows-amd64`, and `windows-arm64`. For example, on Apple Silicon macOS:

```bash
curl -L -o thehivemcp https://github.com/StrangeBeeCorp/TheHiveMCP/releases/latest/download/thehivemcp-darwin-arm64
chmod +x thehivemcp
```

> **macOS:** the binaries are unsigned, so Gatekeeper blocks GUI hosts (VS Code, Claude Desktop) with _"Apple could not verify … is free of malware"_. Clear the
> quarantine attribute once after downloading: `xattr -d com.apple.quarantine ./thehivemcp`. Running from a terminal works without this. GUI hosts don't. To
> skip unsigned binaries entirely, use the Docker path—see [How to set up Claude Code](setup-claude-code.md).
>
> **Windows:** download the matching `windows-*.exe`. The current Windows binaries are unsigned—expect a SmartScreen prompt, and see
> [How to run on Windows from source](run-on-windows-from-source.md) if a signed-only policy blocks it.

### 2. Configure environment

Using the [`.env.template`](../../.env.template):

```bash
# Copy template and configure
cp .env.template .env

# Edit with your TheHive details
THEHIVE_URL=https://your-thehive-instance.com
THEHIVE_API_KEY=your-api-key-here
THEHIVE_ORGANISATION=your-org-name  # Optional, defaults to user's own organisation
PERMISSIONS_CONFIG=read_only
```

## MCP host integration

### GitHub Copilot

Add to your MCP settings:

```jsonc
{
  "mcpServers": {
    "thehive": {
      "command": "/path/to/thehivemcp",
      "args": ["--transport", "stdio"],
      "env": {
        "THEHIVE_URL": "https://your-thehive-instance.com",
        "THEHIVE_API_KEY": "your-api-key-here",
        "THEHIVE_ORGANISATION": "your-org-name", // Optional
        "PERMISSIONS_CONFIG": "read_only",
      },
    },
  },
}
```

### Claude Desktop

Claude Desktop uses the same stdio `command`/`args`/`env` shape. Add the block above to your `claude_desktop_config.json` under `mcpServers`, then restart
Claude Desktop.

For a one-click alternative, install the `.mcpb` bundle instead of wiring stdio by hand—it registers the server and prompts for your TheHive connection
settings. See the [Claude Desktop MCPB path in the README](../../README.md#get-started).

## What to expect

Once configured, your MCP host can:

- **Search entities**: "Find critical alerts from last week"
- **Access resources**: Browse TheHive schemas and documentation
- **Create/modify**: Depends on permissions configuration

## Next steps

- For team deployment: [Remote Docker Example](remote-docker.md)
- For LibreChat integration: [LibreChat Example](librechat.md)
- For custom permissions: [Permissions Guide](../reference/permissions.md)
