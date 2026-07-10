# How to Set Up TheHiveMCP with Claude Code

This guide connects TheHiveMCP to **Claude Code** (the Anthropic CLI tool). Claude Code runs in your terminal and has its own MCP configuration system with a
few non-obvious pitfalls.

There are two ways to run the server locally:

- **Docker (recommended).** It avoids unsigned-binary prompts and works identically from the terminal, VS Code, and any other MCP host. This is the path this
  guide uses.
- **Native binary (alternative).** A single downloaded executable. On macOS, this triggers a Gatekeeper prompt that blocks GUI hosts (VS Code, Claude
  Desktop). See [Alternative: Native Binary](#alternative-native-binary) for the workaround.

## Prerequisites

- Claude Code installed (`claude` CLI available)
- A running TheHive 5.5+ instance with API access
- Your TheHive URL, API key, and organization name
- **Docker** installed and running (for the recommended path)

---

## Step 1 — Verify the server works

Before touching any config, confirm the server starts and answers an MCP `initialize` request over stdio. This one-liner prints a clear **OK** or **FAILED** so
you don't have to read raw JSON-RPC or inspect `$?` by hand:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' \
  | docker run -i --rm \
      -e THEHIVE_URL="<thehive_url>" \
      -e THEHIVE_API_KEY="<thehive_api_key>" \
      -e THEHIVE_ORGANISATION="<thehive_organization>" \
      ghcr.io/strangebeecorp/thehivemcp/thehivemcp:latest /app/server --transport stdio 2>/dev/null \
  | grep -q '"result"' && echo "OK — server responded" || echo "FAILED — see the error below"
```

`OK` confirms the MCP server itself starts and completes the handshake—but the `initialize` step does **not** verify your TheHive credentials (the server
answers the handshake before contacting TheHive). To confirm the credentials too, re-run the command below and look for a credential error in the stderr logs.
A clean run with no `ERROR` line about credentials means the credentials are correct.

If it prints `FAILED`, or to check credentials, re-run without the `grep`/`2>/dev/null` part to read the logs—the server logs to **stderr**:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' \
  | docker run -i --rm \
      -e THEHIVE_URL="<thehive_url>" \
      -e THEHIVE_API_KEY="<thehive_api_key>" \
      -e THEHIVE_ORGANISATION="<thehive_organization>" \
      ghcr.io/strangebeecorp/thehivemcp/thehivemcp:latest /app/server --transport stdio 2>&1 | head -20
```

> **Why `docker run -i`, `/app/server`, and `--transport stdio`?** The `-i` flag keeps stdin open so Claude Code can talk to the container—without it the
> server gets no input and hangs. The image's default command is `/app/server`. Because anything you put after the image name **replaces** that default (the
> image has no `ENTRYPOINT`), you must name the binary explicitly—otherwise Docker tries to execute `--transport` as a program and fails with _"executable
> file not found"_. `--transport stdio` then tells the server to speak MCP over stdio. TheHiveMCP defaults to HTTP, so without it you'd get an HTTP server
> that Claude Code can't reach. `--rm` cleans up the container when the session ends.

---

## Step 2 — Register the server with Claude Code

Use `claude mcp add`. It writes a syntactically valid entry for you, so you never have to hand-edit the large global state file that Claude Code maintains.

```bash
claude mcp add thehive \
  --scope project \
  -- docker run -i --rm \
       -e 'THEHIVE_URL=${THEHIVE_URL}' \
       -e 'THEHIVE_API_KEY=${THEHIVE_API_KEY}' \
       -e 'THEHIVE_ORGANISATION=${THEHIVE_ORGANISATION}' \
       -e 'PERMISSIONS_CONFIG=${PERMISSIONS_CONFIG:-read_only}' \
       ghcr.io/strangebeecorp/thehivemcp/thehivemcp:latest /app/server --transport stdio
```

Everything after `--` is the command Claude Code runs to launch the server. Note the two roles of `-e`:

- The `-e VAR=value` pairs **after `docker run`** are Docker environment flags—they inject the variables **into the container**, where the server reads them.
- Do **not** use the `-e` flag of `claude mcp add` here. That would set the variable in the host process that launches `docker`, not inside the container, so
  the server would never see it.

The `${VAR}` references are wrapped in **single quotes** on purpose: that stops your shell from expanding them at `add` time, so the literal `${VAR}` string is
written into `.mcp.json`. Claude Code then expands it from your environment each time it launches the server (`${VAR:-default}` supplies a fallback). The
result is a `.mcp.json` you can safely commit—export `THEHIVE_URL`, `THEHIVE_API_KEY`, and `THEHIVE_ORGANISATION` in your shell before launching `claude`. To
bake the literal values into the file instead, drop the single quotes and write them directly—but then don't commit the file.

### Choosing a scope

`--scope` decides where the entry is written:

| Scope             | File                          | Use when                                                       |
| ----------------- | ----------------------------- | -------------------------------------------------------------- |
| `project`         | `./.mcp.json`                 | **Recommended.** Small, versionable, shareable with your team. |
| `user`            | `~/.claude.json`              | You want it available in every directory on your machine.      |
| `local` (default) | project-scoped, not committed | Just you, just this project, not checked into git.             |

> **Prefer `--scope project`.** It creates a small `.mcp.json` in the current directory that you can read, diff, and commit. The `user` scope writes into
> `~/.claude.json`, which also holds unrelated Claude Code state (startup count, history, and more) and is awkward to edit or review by hand.

<details>
<summary>Equivalent manual <code>.mcp.json</code> (if you prefer not to use the CLI)</summary>

Create `.mcp.json` in your project directory:

```json
{
  "mcpServers": {
    "thehive": {
      "type": "stdio",
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e",
        "THEHIVE_URL=${THEHIVE_URL}",
        "-e",
        "THEHIVE_API_KEY=${THEHIVE_API_KEY}",
        "-e",
        "THEHIVE_ORGANISATION=${THEHIVE_ORGANISATION}",
        "-e",
        "PERMISSIONS_CONFIG=${PERMISSIONS_CONFIG:-read_only}",
        "ghcr.io/strangebeecorp/thehivemcp/thehivemcp:latest",
        "/app/server",
        "--transport",
        "stdio"
      ]
    }
  }
}
```

> **Keep secrets out of the file: use `${VAR}` references.** Claude Code expands environment variables in `.mcp.json` before launching the server: `${VAR}`
> takes the value from your shell environment, and `${VAR:-default}` falls back to `default` when the variable is unset. Expansion works in `command`,
> `args`, `env`, and (for HTTP servers) `url` and `headers`, in every scope. This lets you commit `.mcp.json` while keeping your API key in your environment
> instead of in git. Export the variables before launching `claude` (for example, in your shell profile or a local `.env` file you source):
>
> ```bash
> export THEHIVE_URL="<thehive_url>"
> export THEHIVE_API_KEY="<thehive_api_key>"
> export THEHIVE_ORGANISATION="<thehive_organization>"
> ```
>
> If a referenced variable is unset and has no default, Claude Code fails to parse the config. You can still write the values inline instead—but then never
> commit the file.

</details>

> **Note:** The `mcpServers` key in `~/.claude/settings.json` is **not** read by Claude Code for MCP discovery. Don't put your server config there.

---

## Step 3 — Restart Claude Code and verify

Restart Claude Code completely (exit with `/exit`, then relaunch `claude`). MCP servers connect at startup—a running session won't pick up config changes.

> **Project-scoped servers need a one-time approval.** The first time you launch `claude` in a directory that has a `.mcp.json`, Claude Code prompts you to
> approve the project's MCP servers before running them (a safeguard against executing commands from a cloned repo). Until you approve, `claude mcp list`
> shows the server as `⏸ Pending approval` and `/mcp` won't show it as connected. Approve it at the startup prompt.

After restart, run:

```text
/mcp
```

You should see `thehive` listed as `connected`. If it's disconnected or missing, check the MCP logs:

```bash
ls ~/Library/Caches/claude-cli-nodejs/*/mcp-logs-thehive/
```

Look for `Successfully connected (transport: stdio)`. If you see an error instead, see [Troubleshooting](#troubleshooting).

---

## What to try once connected

```text
Show me high-severity alerts from the last 7 days
What cases are currently open and assigned to me?
Summarize observable activity for case #1234
```

---

## Alternative: Native binary

If you can't or don't want to use Docker, download the binary for your platform from the [README](../../README.md#get-started).

### macOS: Clear the Gatekeeper quarantine

macOS binaries are currently unsigned. When a GUI host (VS Code, Claude Desktop) or Gatekeeper tries to launch the binary, you get:

> "Apple could not verify 'thehivemcp-darwin-arm64' is free of malware…"

Running it directly from the terminal works because the shell bypasses this check—but MCP hosts don't. Remove the quarantine attribute once, after
downloading:

```bash
xattr -d com.apple.quarantine /path/to/thehivemcp-darwin-arm64
chmod +x /path/to/thehivemcp-darwin-arm64
```

(Alternatively: **System Settings → Privacy & Security → "Open Anyway"** after the first blocked launch.) The binary will then launch from any host.

### Register the native binary

```bash
claude mcp add thehive \
  --scope project \
  -e 'THEHIVE_URL=${THEHIVE_URL}' \
  -e 'THEHIVE_API_KEY=${THEHIVE_API_KEY}' \
  -e 'THEHIVE_ORGANISATION=${THEHIVE_ORGANISATION}' \
  -e 'PERMISSIONS_CONFIG=${PERMISSIONS_CONFIG:-read_only}' \
  -- /path/to/thehivemcp --transport stdio
```

Here the `-e` flags belong to `claude mcp add` (there's no container), and they're written into the server's `env` block. As in Step 2, the single-quoted
`${VAR}` references land literally in `.mcp.json` and are expanded from your environment at launch—export the variables in your shell first, and the file
stays safe to commit. Use the **absolute** path to the binary.

---

## Troubleshooting

### Server not listed in `/mcp`

The entry is missing or the config file has a JSON syntax error. Check what Claude Code sees:

```bash
claude mcp list
```

If you edited `.mcp.json` by hand, validate it:

```bash
python3 -c "import json; json.load(open('.mcp.json')); print('OK')"
```

### `Executable not found` / server won't start (Docker)

- Confirm Docker is running: `docker info`.
- Confirm the image is present or pullable: `docker pull ghcr.io/strangebeecorp/thehivemcp/thehivemcp:latest`.
- Re-run the Step 1 verification command—it isolates whether the problem is Docker, credentials, or the Claude Code configuration.

### Gatekeeper prompt keeps appearing (native binary, macOS)

The quarantine attribute is still set. Re-run `xattr -d com.apple.quarantine /path/to/thehivemcp-darwin-arm64`, or use Docker instead.

### Authentication error on startup

Credentials are wrong or TheHive is unreachable. Run the Step 1 command with `2>&1 | head -20` to read the server's stderr and fix the credentials before
retrying.

### Duplicate or broken entry precedence

An entry in `~/.claude.json` (user scope) takes precedence over `.mcp.json` (project scope). If a stale user-scope entry shadows your project config, remove
it:

```bash
claude mcp remove thehive --scope user
```
