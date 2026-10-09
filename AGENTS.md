# AGENTS.md

Project guidance for coding agents working in this repository.

## Overview

A Model Context Protocol (MCP) server (Go) exposing TheHive security platform to LLM clients. Entry point: `cmd/server/main.go`.
It builds an MCP server (`bootstrap.GetMCPServerAndRegisterTools`) and serves it over either `stdio` or `http` transport, selected by config.

## Architecture

- `bootstrap/` — server construction, tool registration, transports (`stdio.go`, `http.go`), auth middleware, URL allowlist, validation cache.
- `internal/tools/` — the MCP **tools**, grouped by capability: `manage/` (CRUD), `search/`, `resource/`, `execute_automation/`. Shared middlewares in `middlewares.go`.
- `internal/resources/` — MCP **resources** (static + dynamic) and their `facts/`, `rules/`, `schemas/`; registered via `resource_registry.go`.
- `internal/{types,permissions,auth,logging,utils,testutils}/` — config/options, RBAC, auth, logging, helpers, and the integration-test container harness.
- `cmd/harness/`, `internal/harness/`, `gates/`, `bin/harness` — the harness: the gates every change must pass (see below).
- `spec/items.yaml` — what the server promises a model, each item traced to the tests and gates that prove it. `baselines/surface/` is the snapshot of what
  the model reads (`cmd/surface`, `make surface-propose`), and `internal/analyzers/` the Go analyzers of the model-facing rules.

## Main DevEx/AgentEx entrypoint (commands)

Run `make help-xml` to get all available commands, their purpose, and how to use them.
Start by running `make help-xml` at the start of any session that will modify code.

All targets run in Docker; no local Go install needed. The one exception is `make dev`, a hot-reloading server that needs a host Go and `air`.

While you work, run `make check-fast`: it runs the gates your changed paths select, in about a minute. The gates, their cadences and what makes each
one block are listed in `docs/reference/gates.md`, generated from `gates/registry.yaml`. The git pre-commit hook runs `make pre-commit`
(`make dev-setup` enables it), CI runs the same gates through `bin/harness`, and `make gate ID=<gate>` runs one of them on demand. `make pr` runs the pr
gates at the exact commit, records the evidence and pushes the branch; nothing merges a pull request but a person.

When running commands, never pipe its output through a small `tail`/`head` that discards most of it — you lose the full result and must re-run the whole slow suite.
Redirect stdout+stderr to a temp file (e.g. `make test-integration > /tmp/integration.log 2>&1`), then `grep`/inspect that file afterwards.

## Testing

### Integration testing

Integration tests are gated through a single chokepoint: `testutils.StartTheHiveContainer` calls `t.Skip` when `testing.Short()` is set.
Any test that needs a live TheHive instance goes through this helper (directly, or via `SetupTestWithCleanup` / `GetMCPTestClient*`), so it is skipped automatically under `-short` — no per-test build tags or skip guards needed.

A test that does **not** require a container will run under `make test` and must therefore stay fast and deterministic (cacheable).

### What the gates hold tests to

At pr and in CI, a changed test may not get weaker (the integrity gate), each package keeps its coverage floor (`gates/coverage.yaml`) and, where it has one,
its mutation floor (`gates/mutation.yaml`, gremlins), the unit tests run under the race detector, and the hot paths' benchmarks stay within
`gates/bench.yaml`. `.claude/skills/develop-feature/references/testing.md` says what each one asks.

## Configuration

Config comes from env vars (see `.env.template`) or CLI flags; flags override env vars. `.env` is auto-loaded at startup.
Keys defined in `internal/types/constants.go`, parsed in `internal/types/options.go`.
Never read `.env` files (any `**/.env*`); such tool calls are discarded.

- TheHive target: `THEHIVE_URL`, `THEHIVE_API_KEY` (or `THEHIVE_USERNAME` / `THEHIVE_PASSWORD`), `THEHIVE_ORGANISATION`, `THEHIVE_URL_ALLOWLIST`.
- Transport: `stdio` (default) or `http`; http mode requires `MCP_BIND_HOST`
  - `MCP_PORT` (also `MCP_SERVER_ENDPOINT`, `MCP_HEARTBEAT_INTERVAL`).
- `LOG_LEVEL` controls logging.
