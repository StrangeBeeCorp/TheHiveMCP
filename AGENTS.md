# AGENTS.md

Project guidance for coding agents working in this repository.

## Overview

A Model Context Protocol (MCP) server (Go) exposing TheHive security platform to LLM clients. Entry point: `cmd/server/main.go`.
It builds an MCP server (`bootstrap.GetMCPServerAndRegisterTools`) and serves it over either `stdio` or `http` transport, selected by config.

## Architecture

- `bootstrap/` — server construction, tool registration, transports (`stdio.go`, `http.go`), auth middleware, URL allowlist, validation cache.
- `internal/tools/` — the MCP **tools**, grouped by capability: `manage/` (CRUD), `search/`, `resource/`, `execute_automation/`. Shared middlewares in `middlewares.go`.
- `internal/resources/` — MCP **resources** (static + dynamic) and their `facts/`, `rules/`, `schemas/`; registered via `resource_registry.go`.
- `internal/confirmation/` — the approval gate for mutating operations; `Prompter` isolates how the question reaches the MCP client.
- `internal/{types,permissions,auth,logging,utils,testutils}/` — config/options, RBAC, auth, logging, helpers, and the integration-test container harness.

## Main DevEx/AgentEx entrypoint (commands)

Run `make help-xml` to get all available commands, their purpose, and how to use them.
Start by running `make help-xml` at the start of any session that will modify code.

All targets run in Docker; no local Go install needed.

When running commands, never pipe its output through a small `tail`/`head` that discards most of it — you lose the full result and must re-run the whole slow suite.
Redirect stdout+stderr to a temp file (e.g. `make test-integration > /tmp/integration.log 2>&1`), then `grep`/inspect that file afterwards.

## Testing

### Integration testing

Integration tests are gated through a single chokepoint: `testutils.StartTheHiveContainer` calls `t.Skip` when `testing.Short()` is set.
Any test that needs a live TheHive instance goes through this helper (directly, or via `SetupTestWithCleanup` / `GetMCPTestClient*`), so it is skipped automatically under `-short` — no per-test build tags or skip guards needed.

A test that does **not** require a container will run under `make test` and must therefore stay fast and deterministic (cacheable).

## Configuration

Config comes from env vars (see `.env.template`) or CLI flags; flags override env vars. `.env` is auto-loaded at startup.
Keys defined in `internal/types/constants.go`, parsed in `internal/types/options.go`.
Never read `.env` files (any `**/.env*`); such tool calls are discarded.

- TheHive target: `THEHIVE_URL`, `THEHIVE_API_KEY` (or `THEHIVE_USERNAME` / `THEHIVE_PASSWORD`), `THEHIVE_ORGANISATION`, `THEHIVE_URL_ALLOWLIST`.
- Transport: `stdio` (default) or `http`; http mode requires `MCP_BIND_HOST`
  - `MCP_PORT` (also `MCP_SERVER_ENDPOINT`, `MCP_HEARTBEAT_INTERVAL`).
- `LOG_LEVEL` controls logging.
