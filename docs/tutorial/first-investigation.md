# Your first investigation with TheHiveMCP

In this tutorial we will connect TheHiveMCP to an AI assistant and run our first investigation — entirely in natural language, entirely read-only. By the end
you will have asked the assistant to search your TheHive instance, drill into a case, and inspect its observables, and you will have seen the MCP tools do the
work behind the scenes.

We will stay in the safe, default `read_only` mode the whole time, so nothing in TheHive can be changed. This is a lesson, not a production setup — follow the
steps in order and each one will produce the result described.

## What you will need

- A running **TheHive 5.5+** instance with some existing data (a few cases or alerts to look at).
- Your **TheHive URL**, an **API key**, and your **organisation** name.
- **Claude Desktop** installed ([download](https://claude.ai/download)).

We use Claude Desktop here because it is the quickest host to get running. If you prefer the terminal, the [Claude Code how-to](../how-to/setup-claude-code.md)
reaches the same point.

## Step 1 — Download the server

Download the binary for your platform from the [latest release](https://github.com/StrangeBeeCorp/TheHiveMCP/releases). On Apple Silicon macOS:

```bash
curl -L -o thehivemcp https://github.com/StrangeBeeCorp/TheHiveMCP/releases/latest/download/thehivemcp-darwin-arm64
chmod +x thehivemcp
```

Note the full path to the binary — we will need it in a moment. Run `pwd` in the download directory and keep the result handy.

## Step 2 — Register the server with Claude Desktop

Open Claude Desktop's configuration file — **Settings → Developer → Edit Config** — and add a `thehive` server under `mcpServers`. Replace the four values with
your own:

```json
{
  "mcpServers": {
    "thehive": {
      "command": "/absolute/path/to/thehivemcp",
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

We pass `--transport stdio` because Claude Desktop launches the server as a local child process, and `PERMISSIONS_CONFIG=read_only` so the assistant can only
search and read — never modify. (This is also the default, but we set it explicitly to be sure.)

## Step 3 — Restart and confirm the connection

Quit Claude Desktop completely and reopen it. MCP servers connect at startup, so a restart is required to pick up the new configuration.

Once it reopens, look for the tools icon (🔧) near the message box. Click it — you should see **thehive** listed with four tools: `search-entities`,
`manage-entities`, `execute-automation`, and `get-resource`. Seeing them means the server connected successfully.

If **thehive** is missing, the [Claude Code how-to's troubleshooting section](../how-to/setup-claude-code.md#troubleshooting) covers the same failure modes (the
config shape is identical).

## Step 4 — Ask your first question

In a new conversation, type:

```text
Show me the 5 most recent cases in TheHive.
```

The assistant will call `search-entities` and return a short list of your most recent cases, each with its ID, title, and severity. You have just run a TheHive
query without writing a single filter — the assistant translated your request and TheHiveMCP executed it.

## Step 5 — Drill into one case

Pick a case ID from the list and ask:

```text
What observables are attached to that case? Summarise what they tell us.
```

The assistant will search the case's observables (again through `search-entities`, this time enriched with related data) and summarise them for you. Notice that
it reasons over the _data_ TheHive returned — IP addresses, file hashes, domains — rather than making anything up.

## Step 6 — Explore what else is there

Finally, ask the assistant to look around:

```text
What kinds of things can you search for in my TheHive, and how many high-severity cases are open right now?
```

To answer, the assistant will use `get-resource` to discover the available entity types and schemas, then run a count query. You will get back a plain-language
answer grounded in your live data.

## What we did

We connected TheHiveMCP to an AI assistant and ran a complete read-only investigation loop: list → drill in → explore. Everything went through the four MCP
tools, and nothing was ever modified, because we stayed in `read_only` mode.

## Where to go next

- To let the assistant **create and update** entities safely, learn the permission model in the [permissions reference](../reference/permissions.md) — start restrictive,
  then grant only what a role needs.
- To wire TheHiveMCP into a different host, follow a [deployment how-to](../README.md#how-to-guides) (Claude Code, remote Docker, LibreChat).
- To understand exactly how each tool behaves, read the [tool reference](../reference/tools/search-entities.md).
- To understand the security boundary around untrusted TheHive data and why model choice matters, read the
  [production-ready commitments](../../README.md#-production-ready-what-we-commit-to).
