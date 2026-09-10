# Changelog

All notable changes to TheHiveMCP are documented here. The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Note:** All `0.x` releases (v0.3.4 and earlier) were **beta / pre-release** versions. **v1.0.0 is the first production-ready release** — see its notes
> below.

## [1.1.0] - 2026-09-10

### Added

- **All four tools declare their MCP annotations** (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`), so clients can tell a search from a
  deletion when deciding what to prompt for or auto-approve. `search-entities` and `get-resource` are read-only; `manage-entities` is mutating and destructive
  (`delete` and `merge` are irreversible); `execute-automation` is additionally open-world, being the only tool whose effects escape TheHive into third-party
  services. See [Tool annotations](docs/reference/tools/annotations.md).
- **`search-entities` reports truncation and pages.** Every result now carries `offset`, `hasMore` and, when there is more to read, `nextOffset`; a new `offset`
  parameter (0 by default; `offset + limit` must stay under the 10000-row maximum result window) reads the next window. A full page used to be indistinguishable
  from the end of a result set, so a search capped at the default limit of 10 read as "these are all of them" — `count` has always been the number of rows on
  the page, never the size of the match. Detecting this costs no extra call: the query asks TheHive for one row beyond the limit and drops it. `count=true` is
  unaffected, having no window to page.
- **Automation history is searchable.** `search-entities` accepts two new entity types: `job` (Cortex analyzer runs) and `action` (Cortex responder runs), with
  the same filter DSL, sorting, paging and permission scoping as every other entity. Filter on `analyzerName`, `status`, `startDate`, `cortexId` and more, and
  pass `extra-data: ["report"]` to include an analyzer report. New `hive://schema/job` and `hive://schema/action` resources document the filterable fields.
  These types require TheHive's Cortex connector to be enabled.
- **Cortex runs as related data.** `additional-queries` now expands observables with `jobs` and `actions`, and cases, alerts and tasks with `actions` — the
  direct way to answer "what enrichment already ran on this observable?" without re-running an analyzer.
- **Analyzer discovery by observable type.** `hive://metadata/automation/analyzers` accepts a `dataType` query parameter (for example `?dataType=hash`), which
  Cortex resolves server-side, instead of the caller fetching the whole catalog and sifting it.
- **Paging on the automation catalogs.** Both analyzer and responder catalogs accept `offset` and `limit`, and report `total`, `returned`, `offset` and
  `truncated` so a clipped list is no longer indistinguishable from a complete one.
- **`blockedByPolicy` on the automation catalogs.** An empty catalog now reports **how many** workers the permissions allow-list removed — a count, not a flag,
  so `0` means the allow-list removed nothing. The shipped read-only default blocks every analyzer, so an out-of-the-box catalog was previously
  indistinguishable from "Cortex is not connected" or "this deployment has no analyzers".
- **Unknown query parameters on the automation catalogs are rejected**, instead of being ignored. A misspelling (`?datatype=hash`) or a parameter borrowed from
  the sibling catalog (`?entityType=observable` on the analyzers resource) used to return the whole unfiltered catalog and read as a filtered answer.

- **Entity schemas say which fields can actually be filtered on.** Fields that TheHive describes now carry `filterable` in `hive://schema/<entity>`: `exact`
  (full filter DSL and sorting), `fulltext` (substring matches work, exact equality generally does not), or `no` (returned but not indexed). The key is
  deliberately **absent** where TheHive has no opinion — a field it does not describe, an unrecognised index type, a failed describe call, or an entity with no
  describe endpoint — so do not assume it is always present. A short `filterableLegend` travels with the annotation, including that meaning of absence.

  TheHive indexes only some attributes of an entity, and a filter on an unindexed one is **accepted and returns zero rows** — no error, nothing to distinguish
  it from a genuine empty result. On `pattern` that is 12 of 23 attributes, including the ones most worth filtering (`platforms`, `dataSources`, `detection`),
  so the most useful queries against the MITRE catalogue silently returned nothing. Other entities are affected more mildly: `action` (`objectId`,
  `objectType`), `alert` (`computed.handlingDuration*`, `importDate`), `case`, `task`, `attachment`, `job`.

  The values come from TheHive itself — `/api/v1/describe/<entity>` reports an `indexType` per attribute, which we simply never read. They are fetched once per
  entity type per deployment and cached, and every failure path (unreachable TheHive, an entity with no describe endpoint such as `case-template`, an
  unrecognised index type) leaves the schema exactly as it was rather than guessing.

  ⚠️ **Requires TheHive 5.6+.** On 5.5 the schemas are served exactly as before, with no `filterable` key. TheHive 5.5 does answer `describe`, but omits the
  `cardinality` field that thehive4go's generated model requires (it sends `values`/`labels` instead), so the SDK cannot decode the response and the annotation
  falls open. Tracked in #181; it needs an SDK fix, not a change here.

  Rejecting such a filter outright belongs in TheHive, which already returns a 400 with the valid attribute list for an _unknown_ field; that inconsistency has
  been raised with them separately.

### Changed

- **Corrected the annotations the tools were already advertising.** This was not a blank being filled: `mcp.NewTool` populates the annotations itself and the
  field carries no `omitempty`, so every tool shipped mcp-go's pessimistic defaults — `readOnlyHint: false, destructiveHint: true, openWorldHint: true`. A
  client was therefore told that `search-entities` and `get-resource` may perform destructive updates, which could cost them a confirmation prompt on a plain
  search or exclusion from an auto-approve list.
- **MCP SDK upgraded to `mark3labs/mcp-go` v1.0.0**, which implements MCP revision `2026-07-28`. No client-visible protocol change: the HTTP transport
  deliberately continues to serve the handshake-based revisions only, so clients negotiate exactly as before. Serving `2026-07-28` is a separate, later change.
  See [ADR-0003](docs/explanation/adr/0003-track-mcp-2026-07-28-on-mark3labs-mcp-go.md).
- **Automation catalog responses are now an object, not a bare array.** `hive://metadata/automation/analyzers` and `hive://metadata/automation/responders`
  return `{kind, total, returned, offset, truncated, blockedByPolicy, workers}`; the previous array is now the `workers` field.
- **Analyzer reports are no longer returned unasked on job searches.** TheHive attaches a partial report to every job row and `exclude_fields` does not suppress
  it, so a chronological page of ten jobs carried ~76 KB of embedded observables — untrusted third-party content nobody requested. `search-entities` now drops
  it unless `extra-data` includes `report`, which cut that same query to ~1.7 KB.
- **The automation catalogs validate their arguments before calling Cortex.** A bad `offset` or `limit` is a deterministic input error, so it no longer costs a
  full catalog fetch — nor surfaces as a connection or authentication error when both are wrong at once.

### Removed

- **Elicitation-based write confirmation has been removed.** ⚠️ **Behaviour change for clients that support elicitation.** **Every modifying operation** now
  executes directly, with no confirmation prompt — the confirmation gated on the HTTP verb rather than the tool, so the affected set is all seven
  `manage-entities` operations (`create`, `update`, `delete`, `comment`, `promote`, `merge`, `apply-template`) plus the two `execute-automation` writes
  (`run-analyzer`, `run-responder`). In practice this affects GitHub Copilot, effectively the only client that implemented the prompt — most clients, Claude
  Desktop included, never displayed one and already executed these operations unconfirmed.

  This also **supersedes the "Destructive operations fail closed" behaviour** introduced in 1.0.0: with no prompt advertised, there is no prompt that can fail
  to complete, so nothing fails closed on that basis. The `elicitation` server capability is no longer advertised.

  **Authorisation is unchanged.** What a caller may do is still governed by the TheHive API key and the permissions configuration; elicitation was an optional
  confirmation layer above both, never the access-control boundary. **Action required** only if you relied on the prompt as a safety net: tighten the
  [permissions configuration](docs/reference/permissions.md) or scope the API key.

  Removed rather than ported because MCP `2026-07-28` drops server-initiated requests, and its replacement (MRTR) would require rebuilding the layer — including
  a signed `requestState`, since the token round-trips through the client — for a feature with almost no client adoption. Reconsidering it is tracked in
  [#170](https://github.com/StrangeBee/TheHiveMCP/issues/170).

### Fixed

- **Tool input schemas are advertised again.** The mcp-go v1.0.0 upgrade moved schema inference to `github.com/google/jsonschema-go`, which rejects the
  `jsonschema:"enum=...,required=true"` struct-tag syntax the parameter structs used. Inference failed silently — `mcp.WithInputSchema` writes the error to
  stderr and returns without setting a schema — so `manage-entities`, `search-entities` and `execute-automation` each advertised **zero parameters** while
  continuing to work when called correctly. Enumerations and defaults now live in explicit constraints applied on top of inference, and the advertised schemas
  are asserted against their parameter structs so this cannot regress unnoticed.
- **`search-entities` advertises exactly the entity types it accepts.** The advertised enum and the list the handler validates against were two hand-maintained
  slices; both now derive from one, and a test asserts the wire schema agrees with the code path that enforces it. A type accepted by the handler but missing
  from the enum is invisible to clients, which is indistinguishable from unsupported.
- **A requested analyzer report is marked untrusted.** `extra-data: ["report"]` on a job search returned the Cortex report as a plain object, so the
  untrusted-data boundary allowlist — which classifies TheHive's own field names at every nesting depth — emitted attacker-controlled values sitting under
  trusted-looking report keys (`status`, `objectId`, `hashes`) with no boundary tags. The report is now typed `utils.UntrustedSubtree`, as `execute-automation`
  already typed its own (DL-6703).
- **Numeric catalog parameters are validated consistently.** `?limit=1.9` passed as a JSON number was truncated to `1` while the string `"1.9"` was rejected,
  and a non-finite or out-of-range float produced an implementation-defined value. Both forms are now parsed identically.
- **Analyzers could go missing from the catalog.** `hive://metadata/automation/analyzers` fetched a hard-coded first 100 analyzers and applied the permission
  allow-list afterwards, so allowed analyzers positioned beyond that window were silently dropped — a restrictive allow-list could return an empty catalog on a
  Cortex install with more than 100 analyzers. The full catalog is now fetched and permission-filtered before any paging is applied.

- **Every successful `get-resource`, `manage-entities` and `execute-automation` call violated its own advertised output schema.** ⚠️ **This made writes report
  failure while applying.** The three tools return union results that `Unwrap()` flattens to the active variant, hoisting its fields to the top level, while
  `mcp.WithOutputSchema` advertised the _wrapper_ struct — so the declared schema permitted only `resource`/`category`-style wrapper keys and none of the keys
  actually sent. mcp-go v0.43.1 generated those schemas with `AllowAdditionalProperties: true`, which tolerated the extra keys; v1.1.0's upgrade to mcp-go
  v1.0.0 infers with `github.com/google/jsonschema-go`, which emits `additionalProperties: false` and turned the tolerated mismatch into a violation on every
  success path. Because a client may reject a non-conforming result only _after_ the handler has run, mutations and analyzer dispatches committed and were then
  reported as failures — indistinguishable from a rejection, so a retry duplicated the write. The tools now advertise `anyOf` over their variant schemas,
  describing what they actually emit. Error paths were never affected, and `search-entities` was never affected.
- **Date fields were advertised as integers but sent as strings.** `ProcessDatesRecursive` rewrites epoch fields (`_createdAt`, `_updatedAt`, `startDate`, …)
  into formatted strings on the way out, while a schema inferred from the Go struct declared `integer`. Any result carrying a TheHive entity therefore violated
  its schema independently of the union bug. Date-named properties are now typed `["null", "string"]` at every depth, derived from the same field list that
  drives the rewrite.
- **Output schemas are now asserted against real results.** `TestToolSchemas_OutputMatchesAdvertisedSchema` validates a populated result for every union variant
  of all four tools against the schema that tool advertises. Both bugs above passed every existing unit and integration test, because tests assert on the
  payload the code builds and never validate it against the advertised schema — the same blind spot that let the v1.1.0 input-schema regression through, on the
  output side.
- **Observable searches return the IOC.** The default columns for `observable` were `_id`, `dataType` and `_createdAt` — a type and a timestamp, but not the
  indicator. `data` is now a default, so the most common SOC query stops returning rows a caller cannot act on, whether searched directly or expanded through
  `additional-queries`.
- **Dates are ISO 8601 with an offset.** Timestamps rendered as `02-01-2006T15:04:05`: day-first, so `09-10-2026` read as either 9 October or 10 September
  depending on the reader, and offset-free while being formatted in the _server's_ local zone — so a value came back silently shifted with nothing to say so.
  They are now RFC 3339 in UTC (`2023-11-14T22:13:20Z`).
- **`extra-columns` extends the defaults instead of replacing them.** ⚠️ **Behaviour change.** Asking for one more column used to drop `title`, `severity` and
  `status`, so a caller requesting extra data received less of it with nothing to say so. The parameter now behaves the way its name reads: the entity defaults
  are always returned, the requested columns are added to them, and a column named twice is projected once. A caller that previously listed every column it
  wanted still gets them; it now also gets the defaults it used to suppress, so responses for those callers grow.
- **A finished analyzer or responder run no longer invites polling.** `get-job-status` on a `Success` and `get-action-status` on a `Failure` both said "Use
  get-…-status to check for updates", sending an agent to re-poll an answer that cannot change. Terminal states (`Success`, `Failure`, `Deleted`, `Cancelled`)
  now say so.
- **A bad filter field is no longer reported as a permissions problem.** A 400 from an unknown attribute returned "Check that you have permissions to view
  cases". The message now names both possibilities. It deliberately does not promise that the attached response identifies the offending attribute: TheHive does
  return that list, but thehive4go consumes the body while building its own error, so by the time we see it the useful part is usually gone. Raised upstream.
- **The severity scale is documented as configurable.** The `search-entities` description called 1–4 a fixed scale while the entity schemas correctly describe
  the range and labels as configurable per organisation. The description now agrees with the schemas and points at `severityLabel`.
- **`run-responder` documents that it needs the responder's id.** `run-analyzer` accepts a worker name, but a name passed to `run-responder` creates the action
  and then fails inside Cortex with "worker not found". The parameter description now says which identifier to pass and where to get it. Resolving the name
  server-side, so both operations behave alike, is left as a follow-up: it needs a live Cortex to verify.

- **API errors no longer dump the raw HTTP response.** Fourteen call sites formatted `*http.Response` with `%v`, so a failure returned pointer addresses, the
  transport's headers and the server's identity and version (`Server: nginx/…`) instead of anything diagnostic — and after the 404 handling was added, that dump
  reached the `hints` of `manage-entities` results too. Errors now carry the status line and whatever the body still holds, via a single
  `utils.DescribeHTTPResponse` helper, capped at 2 KB. Note the underlying `json: unknown field "errors"` comes from thehive4go failing to decode TheHive's
  structured field-validation errors into its own error model; that is upstream and unchanged, but it is now reported as a cause instead of being buried in a
  struct dump.
- **The last row of the result window is reachable.** `offset=9990&limit=10` was rejected for "reading past the maximum result window" when the page itself ends
  exactly on the last readable row. The truncation probe asks for one row beyond the page, and that extra row — not the page — crossed the boundary. The probe
  is now clamped to the window, so a page ending on row 9999 is served and simply reports `hasMore: false`, which is the only answer it could have had.
- **`count=true` no longer fails the paging-window check.** A count reads no window, so `count=true&offset=9995` was rejected with a message advising the caller
  to use `count=true` — which is what they had done.

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

[1.1.0]: https://github.com/StrangeBee/TheHiveMCP/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/StrangeBee/TheHiveMCP/compare/v0.3.4...v1.0.0
[0.3.4]: https://github.com/StrangeBee/TheHiveMCP/compare/v0.3.3...v0.3.4
