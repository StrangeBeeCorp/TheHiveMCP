# How to Set Up TheHiveMCP with Claude Code

This guide walks you through connecting TheHiveMCP to **Claude Code** (the Anthropic CLI tool). Claude Code is distinct from Claude Desktop — it runs in your
terminal and has its own MCP configuration system with a few non-obvious pitfalls.

## Prerequisites

- Claude Code installed (`claude` CLI available)
- A running TheHive 5.5+ instance with API access
- Your TheHive URL, API key, and organisation name
- TheHiveMCP binary for your platform (see [README](../../README.md#get-started))

---

## Step 1 — Verify the binary works

Before touching any config, confirm the binary starts and exits cleanly in stdio mode:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' \
  | THEHIVE_URL="https://your-thehive-instance.com" \
    THEHIVE_API_KEY="your-api-key" \
    THEHIVE_ORGANISATION="your-org" \
    /path/to/thehivemcp --transport stdio 2>/dev/null
```

You should see a JSON-RPC `initialize` response on stdout. If you get an authentication error, fix your credentials before continuing.

> **Why `--transport stdio`?** TheHiveMCP defaults to HTTP transport. Without this flag, it starts an HTTP server instead of reading from stdin — and Claude
> Code will never get a response.

---

## Step 2 — Understand Claude Code's config file hierarchy

Claude Code reads MCP server configuration from two locations, in order of precedence:

| File             | Scope                             | Written by                   |
| ---------------- | --------------------------------- | ---------------------------- |
| `~/.claude.json` | Global user-level                 | `claude mcp add` CLI command |
| `.mcp.json`      | Project-level (current directory) | You, manually                |

> **Important:** The `mcpServers` key in `~/.claude/settings.json` is **not** read by Claude Code for MCP server discovery. Do not put your server config there.

If an entry exists in both files, `~/.claude.json` takes precedence over `.mcp.json`.

---

## Step 3 — Add the server to `~/.claude.json`

Edit `~/.claude.json` directly. The file contains other Claude Code state (startup count, install method, etc.) — only add or modify the `mcpServers` key.

```json
{
  "mcpServers": {
    "thehive": {
      "type": "stdio",
      "command": "/path/to/thehivemcp",
      "args": ["--transport", "stdio"],
      "env": {
        "THEHIVE_URL": "https://your-thehive-instance.com",
        "THEHIVE_API_KEY": "your-api-key",
        "THEHIVE_ORGANISATION": "your-org",
        "PERMISSIONS_CONFIG": "read_only"
      }
    }
  }
}
```

Replace `/path/to/thehivemcp` with the absolute path to your binary (for example, `~/Downloads/thehivemcp-darwin-arm64` or the path where you installed it).

> **Do not use shell variables in `env` values.** JSON does not expand `$VAR` — the literal string `$THEHIVE_API_KEY` will be passed to the binary, not the
> value of the variable. Write credentials directly, or use a wrapper script.

---

## Step 4 — Restart Claude Code and verify

Restart Claude Code completely (exit with `/exit`, then relaunch `claude`). MCP servers are connected at startup — a `/mcp` reload won't pick up config changes
made while the session was running.

After restart, run:

```text
/mcp
```

You should see `thehive` listed with status `connected`. If it shows as disconnected or missing, check the MCP logs:

```bash
ls ~/Library/Caches/claude-cli-nodejs/*/mcp-logs-thehive/
```

Look for a file containing `Successfully connected (transport: stdio)`. If instead you see an error, the most common causes are listed in the troubleshooting
section below.

---

## Troubleshooting

### Server not listed in `/mcp`

The `mcpServers` key is missing from `~/.claude.json`, or the file has a JSON syntax error. Validate it:

```bash
python3 -c "import json; json.load(open('/Users/$USER/.claude.json')); print('OK')"
```

### `Executable not found` or `command: " "`

You previously ran `claude mcp add` with incorrect arguments, which wrote an entry with a space as the command. Check `~/.claude.json` for entries with
`"command": " "` and fix or remove them. Entries in `~/.claude.json` take precedence over everything else.

### Authentication error on startup

The binary logs go to stderr. Run it manually to see the error:

```bash
THEHIVE_URL="..." THEHIVE_API_KEY="..." /path/to/thehivemcp --transport stdio 2>&1 | head -20
```

## Using `claude mcp add` (alternative)

Instead of editing `~/.claude.json` manually, you can use the CLI:

```bash
claude mcp add thehive \
  -e THEHIVE_URL="https://your-thehive-instance.com" \
  -e THEHIVE_API_KEY="your-api-key" \
  -e THEHIVE_ORGANISATION="your-org" \
  -e PERMISSIONS_CONFIG="read_only" \
  -- /path/to/thehivemcp --transport stdio
```

This writes to `~/.claude.json`. If you later edit `~/.claude.json` manually, be aware that running `claude mcp add thehive` again will overwrite your entry.

---

## What to try once connected

```text
Show me high-severity alerts from the last 7 days
What cases are currently open and assigned to me?
Summarise observable activity for case #1234
```
