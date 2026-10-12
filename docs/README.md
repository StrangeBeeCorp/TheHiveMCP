# TheHiveMCP Documentation

TheHiveMCP is a Model Context Protocol server that lets an AI assistant operate [TheHive](https://strangebee.com/thehive/) — searching cases and alerts,
managing entities, and running Cortex automation — through natural language. For the project overview and install paths, start at the
[main README](../README.md). This directory holds the deeper documentation, organized by what you are trying to do.

## Tutorial

Start here if you are new. A guided, hands-on lesson with a single happy path.

- **[Your first investigation with TheHiveMCP](tutorial/first-investigation.md)** — connect the server to an AI assistant and run a complete read-only
  investigation, from listing cases to inspecting their observables.

## How-to guides

Goal-oriented recipes for a specific task. They assume you already know the basics.

- **[Set up TheHiveMCP with Claude Code](how-to/setup-claude-code.md)** — connect the server to the Anthropic CLI, with config precedence and troubleshooting.
- **[Run on Windows from source](how-to/run-on-windows-from-source.md)** — compile the server locally to sidestep the SmartScreen "unknown publisher" prompt.
- **[Work with the harness](how-to/work-with-the-harness.md)** — take a change from the first edit to the pull request: `make check-fast`, the Claude Code
  hooks, `make pr`.
- **[Handle a CVE](how-to/handle-a-cve.md)** — what to do with a Renovate `security` pull request, a failing `vulncheck` gate, or a CVE in an image.
- **[Configure permissions](how-to/configure-permissions.md)** — start the server with a permission profile, in each deployment mode, and troubleshoot a refused
  call.
- **[Run the unsigned binary on macOS](how-to/run-the-unsigned-binary-on-macos.md)** — clear the Gatekeeper quarantine so a GUI host can launch the binary.
- **[Cut a release](../RELEASING.md)** — the re-test policy, release notes and checklist for a vX.Y.Z tag.
- **[Develop and test against a shared TheHive](how-to/shared-thehive.md)** — run the dev server and the integration suite in one organisation of a long-running
  TheHive instead of booting the test stack.

Worked deployment examples for specific hosts:

- **[stdio — local MCP host integration](how-to/stdio-local.md)** — GitHub Copilot, Claude Desktop, and other local clients.
- **[Remote Docker — HTTP deployment](how-to/remote-docker.md)** — run the server as a remote HTTP service behind a reverse proxy.
- **[Open WebUI](how-to/open-webui.md)** — a web chat with TheHiveMCP as its MCP tool server.

## Reference

Information-oriented, neutral description of the tools and their contracts. Look things up here.

- **[search-entities](reference/tools/search-entities.md)** — search TheHive with the filter DSL.
- **[manage-entities](reference/tools/manage-entities.md)** — create, update, delete, comment, promote, merge, and apply templates.
- **[execute-automation](reference/tools/execute-automation.md)** — run Cortex analyzers and responders and check their status.
- **[get-resource](reference/tools/get-resource.md)** — browse schemas, metadata, and docs through the `hive://` resource system.
- **[Tool annotations](reference/tools/annotations.md)** — the `readOnlyHint`/`destructiveHint`/`idempotentHint`/`openWorldHint` each tool advertises.
- **[Permissions](reference/permissions.md)** — the permission profile format, tool filters, and the enforcement guarantees.
- **[Permission profile examples](reference/permissions-examples/)** — the `read-only`, `analyst` and `admin` YAML profiles
  ([read-only.yaml](reference/permissions-examples/read-only.yaml), [analyst.yaml](reference/permissions-examples/analyst.yaml),
  [admin.yaml](reference/permissions-examples/admin.yaml)).
- **[Configuration reference](reference/configuration.md)** — environment variables, flags, HTTP headers, their priority, and the built-in permission profiles.
- **[Make targets](reference/make-targets.md)** — the Makefile's targets, grouped: build, run and test, quality and security, the harness, Docker, dependencies.
- **[Gates](reference/gates.md)** — every check the harness runs, when it runs and what makes it fail (generated from `gates/registry.yaml`).
- **[The nightly run and the gate-time notes](reference/nightly.md)** — what the nightly run checks, the metric thresholds, and the format of the notes
  `make pr` records.

## Explanation

Understanding-oriented discussion of the reasoning behind the project.

- **[Why the tools carry these annotations](explanation/tool-annotations.md)** — why each tool advertises its `readOnlyHint`, `destructiveHint`,
  `idempotentHint` and `openWorldHint`, and why annotations are not permissions.
- **[The untrusted-data boundary](explanation/untrusted-data-boundary.md)** — why TheHive field values returned to the model are wrapped, deny-by-default.
- **[ADR-0001 — Accuracy and security testing policy](explanation/adr/0001-security-and-accuracy-testing-policy.md)** — what's tested, how, and why.
- **[ADR-0002 — Two scope-resolution paths](explanation/adr/0002-scope-resolution-two-paths.md)** — why the permission scope check resolves user-supplied IDs
  and expansion hits differently.
- **[ADR-0003 — Track MCP 2026-07-28 on mark3labs/mcp-go v1.0.0](explanation/adr/0003-track-mcp-2026-07-28-on-mark3labs-mcp-go.md)** — why the spec revision is
  a dependency bump rather than a migration to the official Go SDK.
- **[ADR-0004 — The harness judges what changed, not the commands that changed it](explanation/adr/0004-judge-outcomes.md)** — why the agent harness leaves
  commands to Claude Code and judges the files an agent changed.
- **[Evaluation evidence](evaluation/README.md)** — published per-model accuracy and prompt-injection resilience results, per server version.
