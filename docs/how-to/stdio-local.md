# stdio Example - Local MCP Host Integration

This example shows how to run TheHiveMCP in stdio mode for integration with local MCP hosts like GitHub Copilot and Claude Desktop.

## Prerequisites

- TheHive 5.5+ instance with API access
- TheHive API key (and optionally an organisation name)
- MCP host that supports stdio transport

## Setup

### 1. Download binary

Pick the archive matching your OS and architecture from the [releases page](https://github.com/StrangeBeeCorp/TheHiveMCP/releases):
`thehivemcp-<version>-<target>.tar.gz`, for `darwin-amd64`, `darwin-arm64`, `linux-amd64`, `linux-arm64`, `windows-amd64`, and `windows-arm64`. Each holds the
binary alone. For example, on Apple Silicon macOS:

```bash
VERSION=v1.2.2  # the latest tag on the releases page
curl -LO "https://github.com/StrangeBeeCorp/TheHiveMCP/releases/download/${VERSION}/thehivemcp-${VERSION}-darwin-arm64.tar.gz"
tar xzf "thehivemcp-${VERSION}-darwin-arm64.tar.gz"
mv thehivemcp-darwin-arm64 thehivemcp
```

> **macOS:** the binaries are unsigned, so Gatekeeper blocks GUI hosts (VS Code, Claude Desktop). The steps that clear it are in
> [Run the unsigned binary on macOS](run-the-unsigned-binary-on-macos.md). To skip unsigned binaries entirely, use the Docker path—see
> [How to set up Claude Code](setup-claude-code.md).
>
> **Windows:** download the matching `windows-*.tar.gz` and extract it with `tar xzf` (built into PowerShell); it holds `thehivemcp-windows-<arch>.exe`. The
> current Windows binaries are unsigned—expect a SmartScreen prompt, and see [How to run on Windows from source](run-on-windows-from-source.md) if a signed-only
> policy blocks it.

### 2. Configure environment

An MCP host starts the binary itself, so pass the settings in the `env` block of the host's configuration, as below. A `.env` file is only read from the
directory the binary starts in, which the host chooses. Every variable, flag and header is listed in
[the configuration reference](../reference/configuration.md).

## MCP host integration

### GitHub Copilot (VS Code)

Add to `.vscode/mcp.json` in your workspace, or run **MCP: Add Server** from the command palette:

```jsonc
{
  "servers": {
    "thehive": {
      "type": "stdio",
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

Claude Desktop uses the same `command`/`args`/`env` shape, under `mcpServers` instead of `servers` and without `type`. Add it to your
`claude_desktop_config.json`, then restart Claude Desktop.

For a one-click alternative, install the `.mcpb` bundle instead of wiring stdio by hand—it registers the server and prompts for your TheHive connection
settings. See the [Claude Desktop MCPB path in the README](../../README.md#get-started).

## What to expect

Once configured, your MCP host can:

- **Search entities**: "Find critical alerts from last week"
- **Access resources**: Browse TheHive schemas and documentation
- **Create/modify**: Depends on permissions configuration

## Next steps

- For team deployment: [Remote Docker Example](remote-docker.md)
- For a web chat: [Open WebUI](open-webui.md)
- For custom permissions: [Configure permissions](configure-permissions.md)
