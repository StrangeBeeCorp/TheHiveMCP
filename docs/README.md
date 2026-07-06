# TheHiveMCP documentation

TheHiveMCP is a Model Context Protocol server that lets an AI assistant operate [TheHive](https://strangebee.com/thehive/) — searching cases and alerts,
managing entities, and running Cortex automation — through natural language. For the project overview, install paths, and configuration reference, start at the
[main README](../README.md). This directory holds the deeper documentation, organised by what you are trying to do.

## Tutorial

Start here if you are new. A guided, hands-on lesson with a single happy path.

- **[Your first investigation with TheHiveMCP](tutorial/first-investigation.md)** — connect the server to an AI assistant and run a complete read-only
  investigation, from listing cases to inspecting their observables.

## How-to guides

Goal-oriented recipes for a specific task. They assume you already know the basics.

- **[Set up TheHiveMCP with Claude Code](how-to/setup-claude-code.md)** — connect the server to the Anthropic CLI, with config precedence and troubleshooting.
- **[Run on Windows from source](how-to/run-on-windows-from-source.md)** — compile the server locally to sidestep the SmartScreen "unknown publisher" prompt.

Worked deployment examples for specific hosts:

- **[stdio — local MCP host integration](examples/stdio-local.md)** — GitHub Copilot, Claude Desktop, and other local clients.
- **[Remote Docker — HTTP deployment](examples/remote-docker.md)** — run the server as a remote HTTP service behind a reverse proxy.
- **[LibreChat integration](examples/librechat.md)** — a complete web chat stack using Claude models.

## Reference

Information-oriented, neutral description of the tools and their contracts. Look things up here.

- **[search-entities](tools/search-entities.md)** — search TheHive with the filter DSL.
- **[manage-entities](tools/manage-entities.md)** — create, update, delete, comment, promote, merge, and apply templates.
- **[execute-automation](tools/execute-automation.md)** — run Cortex analyzers and responders and check their status.
- **[get-resource](tools/get-resource.md)** — browse schemas, metadata, and docs through the `hive://` resource system.
- **[Permissions](permissions.md)** — the permission profile format, tool filters, and the untrusted-data boundary.
- **[Configuration parameters](../README.md#configuration)** — environment variables, flags, and HTTP headers (in the main README).

## Explanation

Understanding-oriented discussion of the reasoning behind the project.

- **[ADR-0001 — Accuracy and security testing policy](explanation/adr/0001-security-and-accuracy-testing-policy.md)** — what we test, how, and why.
- **[Evaluation evidence](evaluation/README.md)** — published per-model accuracy and prompt-injection resilience results, per server version.
