# Make targets

The Makefile's targets, grouped by what they do. All development operations use Docker containers for consistency and isolation; `make help` lists every target
with its own description. The gates these targets feed are in [the gates reference](gates.md), and [Work with the harness](../how-to/work-with-the-harness.md)
walks through a change from the first edit to the pull request.

## Build, run and test

- `make all` - The pr gates on the whole branch, as `make pr` runs them, without recording or pushing
- `make build` - Build binary using Docker
- `make run ARGS="arguments"` - Run application with custom arguments
- `make test` - Run fast unit tests only (skips integration tests via `-short`); results are cached, so a second run is near-instant
- `make test-integration` - Run the full suite against the docker-compose test stack (`docker-compose.test.yml`); requires Docker with compose
- `make dev` - Development server with hot reload (requires local air)

## Quality and security

- `make fmt` - Format code in Docker (`bin/harness fmt`: gofmt, golangci fmt, shfmt, markdownlint, prettier); `bin/harness fmt --check` lists what it would
  rewrite
- `make lint` - Read-only lint over every file (`bin/harness lint`: golangci-lint, `go test -short`, shellcheck/shfmt, markdownlint, lychee, yamllint, hadolint,
  checkmake, actionlint); `make lint-fix` applies the safe fixes
- `make sast`, `make vulncheck`, `make dockerlint`, `make dockersec`, `make secrets` - The security checks one by one; their gates run them at pr and in CI

## The harness

- `make check-fast` - The gates your changed paths select, in about a minute: the loop to run while you work
- `make gate ID=<gate>` - One gate now, whatever changed
- `make pr` - The pr gates at HEAD, recorded, then the push of the branch (never main)
- `make harness-docs` - Regenerate `docs/reference/gates.md` after changing `gates/registry.yaml`
- `make dev-setup` - Point git at the tracked hooks in `.githooks/` (`git config core.hooksPath .githooks`)
- `bin/harness structure` - The largest code files and the fullest directories against the structure limits

## Docker operations

- `make docker-build` - Build production Docker image
- `make docker-run` - Run production container

## Dependencies and utilities

- `make updatedep` - Update Go dependencies
- `make clean` - Remove build artifacts
- `make help` - Display all available targets
