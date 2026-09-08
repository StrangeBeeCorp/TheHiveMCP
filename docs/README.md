# TheHiveMCP Documentation

TheHiveMCP is a Model Context Protocol server that lets an AI assistant operate [TheHive](https://strangebee.com/thehive/) — searching cases and alerts,
managing entities, and running Cortex automation — through natural language. For the project overview, install paths, and configuration reference, start at the
[main README](../README.md). This directory holds the deeper documentation, organized by what you are trying to do.

## Tutorial

Start here if you are new. A guided, hands-on lesson with a single happy path.

- **[Your first investigation with TheHiveMCP](tutorial/first-investigation.md)** — connect the server to an AI assistant and run a complete read-only
  investigation, from listing cases to inspecting their observables.

## How-to guides

Goal-oriented recipes for a specific task. They assume you already know the basics.

- **[Set up TheHiveMCP with Claude Code](how-to/setup-claude-code.md)** — connect the server to the Anthropic CLI, with config precedence and troubleshooting.
- **[Run on Windows from source](how-to/run-on-windows-from-source.md)** — compile the server locally to sidestep the SmartScreen "unknown publisher" prompt.

Worked deployment examples for specific hosts:

- **[stdio — local MCP host integration](how-to/stdio-local.md)** — GitHub Copilot, Claude Desktop, and other local clients.
- **[Remote Docker — HTTP deployment](how-to/remote-docker.md)** — run the server as a remote HTTP service behind a reverse proxy.
- **[LibreChat integration](how-to/librechat.md)** — a complete web chat stack using Claude models.

## Reference

Information-oriented, neutral description of the tools and their contracts. Look things up here.

- **[search-entities](reference/tools/search-entities.md)** — search TheHive with the filter DSL.
- **[manage-entities](reference/tools/manage-entities.md)** — create, update, delete, comment, promote, merge, and apply templates.
- **[execute-automation](reference/tools/execute-automation.md)** — run Cortex analyzers and responders and check their status.
- **[get-resource](reference/tools/get-resource.md)** — browse schemas, metadata, and docs through the `hive://` resource system.
- **[Tool annotations](reference/tools/annotations.md)** — the `readOnlyHint`/`destructiveHint`/`idempotentHint`/`openWorldHint` each tool advertises, and why.
- **[Permissions](reference/permissions.md)** — the permission profile format, tool filters, and the untrusted-data boundary.
- **[Configuration parameters](../README.md#configuration)** — environment variables, flags, and HTTP headers (in the main README).

## Explanation

Understanding-oriented discussion of the reasoning behind the project.

- **[ADR-0001 — Accuracy and security testing policy](explanation/adr/0001-security-and-accuracy-testing-policy.md)** — what's tested, how, and why.
- **[ADR-0002 — Two scope-resolution paths](explanation/adr/0002-scope-resolution-two-paths.md)** — why the permission scope check resolves user-supplied IDs
  and expansion hits differently.
- **[ADR-0003 — Track MCP 2026-07-28 on mark3labs/mcp-go v1.0.0](explanation/adr/0003-track-mcp-2026-07-28-on-mark3labs-mcp-go.md)** — why the spec revision is
  a dependency bump rather than a migration to the official Go SDK.
- **[Evaluation evidence](evaluation/README.md)** — published per-model accuracy and prompt-injection resilience results, per server version.
