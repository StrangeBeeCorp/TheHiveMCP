# AGENTS.md

Project guidance for coding agents working in this repository.

## Overview

A Model Context Protocol (MCP) server (Go) exposing TheHive security platform
to LLM clients. Entry point: `cmd/server/main.go`. It builds an MCP server
(`bootstrap.GetMCPServerAndRegisterTools`) and serves it over either `stdio`
or `http` transport, selected by config.

## Architecture

- `bootstrap/` — server construction, tool registration, transports
  (`stdio.go`, `http.go`), auth middleware, URL allowlist, validation cache.
- `internal/tools/` — the MCP **tools**, grouped by capability:
  `manage/` (CRUD), `search/`, `resource/`, `execute_automation/`. Shared
  middlewares in `middlewares.go`.
- `internal/resources/` — MCP **resources** (static + dynamic) and their
  `facts/`, `rules/`, `schemas/`; registered via `resource_registry.go`.
- `internal/prompts/` — MCP **prompts** (system prompt, filters, templates).
- `internal/{types,permissions,auth,logging,utils,testutils}/` — config/options,
  RBAC, auth, logging, helpers, and the integration-test container harness.

## Common commands

All targets run in Docker; no local Go install needed. `make help` lists all.

- `make build` — build the binary for the host OS/arch.
- `make run ARGS="…"` — build then run with args.
- `make dev` — development server with hot reload (air).
- `make fmt` — format the code.
- `make security` — vulncheck + SAST + vet/lint.
- `make all` — fmt, security, test, build.

## Testing

All test commands run in Docker (see the `Makefile`); no local Go install is
required.

- `make test` — **default; fast unit tests only.** Runs `go test -short`,
  which skips the testcontainers-based integration tests. Results are cached,
  so a second consecutive run is near-instant.
- `make test-integration` — full suite including integration tests. These
  boot a real TheHive container (`strangebee/thehive`) via testcontainers, so
  they are slow (minutes) and cannot be result-cached. Requires Docker network
  and socket access.
- Add `COVERAGE=1` to either target to enable coverage (off by default): it
  writes `coverage.out` and prints a `go tool cover -func` report. For example,
  `make test COVERAGE=1`.

### Writing tests

Integration tests are gated through a single chokepoint:
`testutils.StartTheHiveContainer` calls `t.Skip` when `testing.Short()` is set.
Any test that needs a live TheHive instance goes through this helper (directly,
or via `SetupTestWithCleanup` / `GetMCPTestClient*`), so it is skipped
automatically under `-short` — no per-test build tags or skip guards needed.

A test that does **not** require a container will run under `make test` and must
therefore stay fast and deterministic (cacheable).

## Configuration

Config comes from env vars (see `.env.template`) or CLI flags; flags override
env vars. `.env` is auto-loaded at startup. Keys defined in
`internal/types/constants.go`, parsed in `internal/types/options.go`.

- TheHive target: `THEHIVE_URL`, `THEHIVE_API_KEY` (or `THEHIVE_USERNAME` /
  `THEHIVE_PASSWORD`), `THEHIVE_ORGANISATION`, `THEHIVE_URL_ALLOWLIST`.
- Transport: `stdio` (default) or `http`; http mode requires `MCP_BIND_HOST`
  - `MCP_PORT` (also `MCP_SERVER_ENDPOINT`, `MCP_HEARTBEAT_INTERVAL`).
- `LOG_LEVEL` controls logging.
