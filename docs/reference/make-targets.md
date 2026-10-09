# Make targets

The targets used day to day, grouped by what they do: a selection, not the whole list. `make help` lists every target with its description and usage
(`make help-xml` for agents). Every target runs in a pinned Docker image, except `make dev`. The gates these targets feed are in
[the gates reference](gates.md), and [Work with the harness](../how-to/work-with-the-harness.md) walks through a change from the first edit to the pull request.

## Build, run and test

- `make all` - The pr gates on the whole branch, as `make pr` runs them, without recording or pushing
- `make build` - Build binary using Docker
- `make run ARGS="arguments"` - Run application with custom arguments
- `make test` - Run fast unit tests only (skips integration tests via `-short`); results are cached, so a second run is near-instant
- `make test-race` - The unit tests under the race detector
- `make test-integration` - Run the full suite against the docker-compose test stack (`docker-compose.test.yml`); requires Docker with compose. The stack stays
  up for the next run; `make test-integration-down` removes it
- `make dev` - Development server with hot reload: the one target that runs on the host, with a host Go and `air`

## Quality and security

- `make fmt` - Format code in Docker (`bin/harness fmt`: gofmt, golangci fmt, shfmt, markdownlint, prettier); `bin/harness fmt --check` lists what it would
  rewrite
- `make lint` - Read-only lint over every file (`bin/harness lint`: golangci-lint, `go test -short`, shellcheck/shfmt, markdownlint, lychee, yamllint, hadolint,
  checkmake, actionlint); `make lint-fix` applies the safe fixes
- `make sast`, `make vulncheck`, `make dockersec`, `make secrets` - The security checks one by one; their gates run them at pr and in CI (hadolint runs in
  `make lint`)

## The harness

- `make check-fast` - The gates your changed paths select, in about a minute: the loop to run while you work
- `make gate ID=<gate>` - One gate now, whatever changed
- `make pr` - The pr gates at HEAD, recorded, then the push of the branch (never main)
- `make harness-docs` - Regenerate `docs/reference/gates.md` after changing `gates/registry.yaml`
- `make surface-propose` - Rewrite `baselines/surface/` to what the server now advertises, after a model-facing change
- `make dev-setup` - Point git at the tracked hooks in `.githooks/` (`git config core.hooksPath .githooks`)
- `bin/harness structure` - The largest code files and the fullest directories against the structure limits

## Docker operations

- `make docker-build` - Build production Docker image
- `make docker-run` - Run production container

## Dependencies and utilities

- `make clean` - Remove build artifacts
- `make help` - Display every target, with its usage
