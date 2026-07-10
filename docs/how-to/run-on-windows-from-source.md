# How to Run TheHiveMCP on Windows from Source (with Go)

This guide is for Windows users who want to run TheHiveMCP **without downloading a prebuilt binary**—by compiling it locally with the Go toolchain. Because a
locally compiled binary is never marked as downloaded-from-the-internet, this path sidesteps the SmartScreen "unknown publisher" prompt you would otherwise see
on a released `.exe`.

> **This is a fallback, not the recommended path.** The supported way to run TheHiveMCP on Windows is the released binary / `.mcpb` bundle (see the
> [README](../../README.md#get-started)). Reach for source builds only if you already have the Go toolchain and want to avoid the download step. Note the hard
> limit in [Before You Start](#before-you-start): building from source does **not** bypass Smart App Control / WDAC (Windows Defender Application
> Control)—an unsigned binary is still an unsigned binary at execution time, and the current released Windows binaries are themselves unsigned (see the
> [README](../../README.md#get-started) note).

## Prerequisites

- **Go 1.26 or later** installed and on your `PATH` (`go version` should succeed)
- A running TheHive 5.5+ instance with API access
- Your TheHive URL, API key, and organization name
- An MCP host to connect to (Claude Desktop, Claude Code, or similar)

## Before you start

Building from source clears the _download_ trust gate but not the _execution_ one:

- ✅ **SmartScreen**—a source-built binary carries no Mark-of-the-Web, so the "unknown publisher" download/run prompt doesn't appear.
- ❌ **Smart App Control / WDAC**—on hardened, policy-managed Windows 11 machines configured to run signed code only, an unsigned binary is **blocked at
  launch** regardless of how it was produced. The current released binaries are unsigned too, so no install path bypasses this. If you're on such a machine,
  ask your Windows administrator to allow the binary, or run TheHiveMCP over HTTP from a non-Windows host instead.

If you're unsure whether your machine enforces a signed-only policy, try [Option A](#option-a--build-once-recommended-for-source-builds) first—a hard block
on launch is the symptom.

---

## Option A — build once (recommended for source builds)

Compile a stable `.exe` once, then point your MCP host at it. This is the recommended source path: the binary is built once, has a fixed location, and starts
instantly.

From a checkout of the repository:

```powershell
go build -o thehivemcp.exe ./cmd/server
```

Or install it onto your `PATH` straight from the module proxy, without cloning:

```powershell
go install github.com/StrangeBeeCorp/TheHiveMCP/cmd/server@latest
```

`go install` places the binary in `%GOPATH%\bin` (usually `%USERPROFILE%\go\bin`) named `server.exe`. Rename it to `thehivemcp.exe` if you prefer a clearer
name.

Verify it starts and speaks stdio before wiring it into a host:

```powershell
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' |
  $env:THEHIVE_URL="<thehive_url>";
  $env:THEHIVE_API_KEY="<thehive_api_key>";
  $env:THEHIVE_ORGANISATION="<thehive_organization>";
  .\thehivemcp.exe --transport stdio
```

You should see a JSON-RPC `initialize` response on stdout.

Then configure your host to launch that binary directly—for example, in the `~/.claude.json` file used by Claude Code:

```json
{
  "mcpServers": {
    "thehive": {
      "type": "stdio",
      "command": "C:\\Users\\<username>\\go\\bin\\thehivemcp.exe",
      "args": ["--transport", "stdio"],
      "env": {
        "THEHIVE_URL": "<thehive_url>",
        "THEHIVE_API_KEY": "<thehive_api_key>",
        "THEHIVE_ORGANISATION": "<thehive_organization>",
        "PERMISSIONS_CONFIG": "read_only"
      }
    }
  }
}
```

For the full Claude Code walkthrough (config precedence, verification, and troubleshooting), follow
[How to Set Up TheHiveMCP with Claude Code](setup-claude-code.md) and substitute your source-built path for the binary.

---

## Option B — launch with `go run` (development only)

`go run` compiles to a throwaway binary and executes it in one step. It's convenient while hacking on the server, but **do not configure a host to launch a
server this way for day-to-day use**—see [the caveats](#why-go-run-is-development-only) below.

To try the server directly from a checkout:

```powershell
$env:THEHIVE_URL="<thehive_url>"
$env:THEHIVE_API_KEY="<thehive_api_key>"
$env:THEHIVE_ORGANISATION="<thehive_organization>"
go run ./cmd/server --transport stdio
```

If you want a host to invoke it, the `command` becomes `go` and the package path moves into `args`:

```json
{
  "mcpServers": {
    "thehive": {
      "type": "stdio",
      "command": "go",
      "args": ["run", "./cmd/server", "--transport", "stdio"],
      "env": {
        "THEHIVE_URL": "<thehive_url>",
        "THEHIVE_API_KEY": "<thehive_api_key>",
        "THEHIVE_ORGANISATION": "<thehive_organization>",
        "PERMISSIONS_CONFIG": "read_only"
      }
    }
  }
}
```

The host must launch this from the repository root (so `./cmd/server` resolves), and `go` must be on the host's `PATH`.

### Why `go run` is development-only

- **Compile-on-launch latency.** MCP hosts start—and often restart—the server as a child process. `go run` recompiles on a cold build cache, injecting
  seconds of delay and a toolchain dependency into what should be an instant stdio spawn.
- **No pinned version with `@latest`.** `go run <module_path>@latest` re-resolves to whatever the newest tag is at each launch, fetched over the network.
  Silently floating a security tool to the latest upstream on every start is not a posture you want in production.
- **Still unsigned.** The transient binary is unsigned like any other source build, so the Smart App Control / WDAC block in
  [Before You Start](#before-you-start) applies unchanged.

For anything beyond experimentation, use [Option A](#option-a--build-once-recommended-for-source-builds).

---

## See also

- [How to Set Up TheHiveMCP with Claude Code](setup-claude-code.md): full host configuration walkthrough.
- [README: Get Started](../../README.md#get-started): the supported released-binary / `.mcpb` install path.
