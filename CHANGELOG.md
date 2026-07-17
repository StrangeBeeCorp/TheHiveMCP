# Changelog

All notable changes to TheHiveMCP are documented here. The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Note:** All `0.x` releases (v0.3.4 and earlier) were **beta / pre-release** versions. **v1.0.0 is the first production-ready release** — see its notes
> below.

## [1.0.0] - 2026-07-15

TheHiveMCP is now **production-ready and out of beta**. This release removes the beta warning, publishes accuracy and security evaluation evidence, replaces the
internal LLM in search with a deterministic filter interface, hardens the security envelope, and relicenses the project under Apache 2.0. Read the
[Changed](#changed) and [Removed](#removed) sections below before upgrading—the search interface and the OpenAI-related configuration have changed.

### Added

- **Production-readiness commitments.** The README now includes a **"Production-ready: what we commit to"** section that states the supported deployment shape,
  the recommended-models constraint, the security and accuracy envelopes it vouches for (and their boundaries), and what it explicitly doesn't commit to
  (arbitrary models, untested workloads, and latency/throughput SLAs).
- **Published accuracy and security evaluation evidence** in [`docs/evaluation/`](docs/evaluation/), with per-model results tied to a named MCP-server version.
  [ADR-0001](docs/explanation/adr/0001-security-and-accuracy-testing-policy.md) records the commitment shape and testing policy, and
  [RELEASING.md](RELEASING.md) records the re-test policy that keeps the evidence current. Evidence folders cover v0.3.3, v0.3.4, and v1.0.0.
- **Similarity search.** `search-entities` now exposes TheHive similarity engine through `similarCases` and `similarAlerts`, so you can surface related cases
  and alerts directly. `search-entities` re-scopes similarity hits against your MCP permission filters and documents the new similarity fields in the case and
  alert output schemas.
- **Filter DSL cheatsheet resource.** A new `hive://docs/overview/filter-dsl` resource documents TheHive filter grammar that `search-entities` now accepts,
  alongside the existing `hive://schema/*` resources.
- **Windows release artifacts.** The release pipeline now produces `windows-amd64` and `windows-arm64` binaries, closing a gap where the README and installer
  advertised a Windows download the build never produced.
- **Container health check.** The Docker image now ships a self-contained `/healthcheck` probe and a `HEALTHCHECK` directive, so orchestrators can detect an
  unhealthy server. The port is configurable via `MCP_PORT`.
- **Documentation.** TheHiveMCP documentation now includes a Windows-from-source how-to and `AGENTS.md`. The ADRs now live under
  [`docs/explanation/adr/`](docs/explanation/adr/).

### Changed

- **`search-entities` is now deterministic—provide filters directly.** The tool no longer translates a natural-language query through an internal LLM (MCP
  sampling with an OpenAI fallback). Instead, the caller supplies TheHive filter DSL through a new optional `filters` parameter, which `search-entities` applies
  as-is. This makes searches precise, deterministic, and free of an inner LLM round-trip. An empty `filters` parameter matches all. On invalid input, the tool
  returns actionable errors pointing at the `hive://schema/*` and `hive://docs/overview/filter-dsl` resources. **Action required:** callers that previously
  passed a `query` string must now pass a `filters` object—see the updated `search-entities` documentation.
- **License changed from MIT to Apache 2.0** (Copyright StrangeBee). As the project exits beta, it now ships with an explicit patent grant and
  patent-retaliation clause. This release adds a `NOTICE` file for attribution and updates the declared license metadata accordingly.
- **Go toolchain upgraded to 1.26.4.**
- **Beta warning removed** from the README, the docs site, and the install/example guides.

### Removed

- **OpenAI configuration and the internal LLM plumbing.** With `search-entities` no longer generating filters internally, TheHiveMCP removed the OpenAI client,
  the sampling infrastructure, and the natural-language filter prompts. TheHiveMCP also dropped the leftover `OPENAI_*` variables from `.env.template` and the
  generated MCPB manifest. **Action required:** remove any `OPENAI_*` variables from your configuration—they are no longer read.

### Fixed

- **Malformed search filter keys are now normalized** before being forwarded to TheHive, preventing valid searches from failing on minor key formatting
  differences.
- **Windows download link** in the README/installer no longer points at an artifact that was never built (see the new Windows release targets under
  [Added](#added)).

### Security

TheHiveMCP hardened its security envelope to be **safe by default**. [`docs/evaluation/`](docs/evaluation/) documents the prompt-injection defense
(`[UNTRUSTED_DATA]` boundary tags on all user-generated fields) and its measured resilience per recommended model.

- **HTTP is safe by default.** The server now enforces a TheHive URL allowlist and fails closed on authentication rather than proceeding with an unverified
  connection.
- **Untrusted data is wrapped deny-by-default.** TheHiveMCP boundary-tags user-generated fields—now including custom fields and attachments—deriving the
  trusted-field allowlist from TheHive OpenAPI schema instead of heuristics.
- **Destructive operations fail closed.** When an advertised confirmation prompt (elicitation) cannot be completed, TheHiveMCP denies the operation instead of
  executing it unconfirmed.
- **Entity scoping is enforced end to end.** TheHiveMCP enforces tool filters on entity reach, validates `get-resource` parameters, and requires entity IDs to
  be `~`-prefixed numeric identifiers.

## [0.3.4] - 2026-04-13

_A beta / pre-release version._ A compatibility and hardening release: it adds support for TheHive 5.5+, introduces the first prompt-injection mitigation, and
makes the Cortex analyzer/responder integration configurable.

### Added

- **TheHive 5.5+ compatibility.** Upgraded the TheHive4Go client to 0.56.2 and validated against TheHive 5.6.3, with 5.5+ compatibility documented.
- **Configurable default Cortex ID.** The Cortex instance used by `execute-automation` (analyzers/responders) is no longer hard-coded — you can now set a
  default `cortexId` via configuration. See the README.

### Changed

- **Go toolchain upgraded** through 1.25.9 and 1.25.10.

### Fixed

- **Extra columns in search results.** `search-entities` no longer leaks the internal `_id` system field into results when additional queries are used; it is
  now excluded alongside the other system fields.

### Security

- **Prompt-injection mitigation (initial).** User-generated fields are now wrapped in `[UNTRUSTED_DATA]` boundary tags to help the model distinguish
  instructions from data, scoped to tools that carry user-generated content, with boundary tags escaped inside values so they cannot be spoofed. This is the
  foundation the v1.0.0 security envelope builds on.

[1.0.0]: https://github.com/StrangeBee/TheHiveMCP/compare/v0.3.4...v1.0.0
[0.3.4]: https://github.com/StrangeBee/TheHiveMCP/compare/v0.3.3...v0.3.4
