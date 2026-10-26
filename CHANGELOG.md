# Changelog

All notable changes to TheHiveMCP are documented here. The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Note:** All `0.x` releases (v0.3.4 and earlier) were **beta / pre-release** versions. **v1.0.0 is the first production-ready release** — see its notes
> below.

## [Unreleased]

## [1.2.1] - 2026-10-23

**Not re-evaluated.** This release was not run through the accuracy and security evaluation: v1.2.0's evidence
([docs/evaluation/v1.2.0](docs/evaluation/v1.2.0/summary.md)) stays the latest. The fixes below change what a model reads back from some calls (errors of
responder and analyzer runs, the custom-fields resource, search columns), so its figures may not hold exactly for this version.

### Added

- **`search-entities` can search the knowledge base alone: entity-type `knowledge-base`.** A `page` search returns the knowledge base mixed with every case's
  pages, and nothing on a row says which kind it is: in one organisation, 1 of 16 pages came from the knowledge base, so an agent looking for a dictionary page
  had to guess its title. `knowledge-base` runs TheHive's `listOrganisationPage` query. Its rows are pages, updated and deleted through `manage-entities` with
  `entity-type: page` when the permission profile allows it.

### Changed

- **Built on the TheHive client generated from the TheHive 6.0 spec (thehive4go v0.60.0), and tested on TheHive 5.5, 5.8 and 6.0.** The client was generated
  from the 5.6 spec, so an answer carrying an enum value it did not list (a permission, a field type or a user type TheHive added since) failed to decode and
  reached the model as an error; enums in answers now keep a value they do not list. CI runs the integration suite on TheHive 5.5.16, 5.8.0 and 6.0.0, each with
  a Cortex, so the analyzer and responder paths run on every version. Search rows still carry only the columns asked for, now that the client's models list the
  fields 5.8 and 6.0 added (a task's `timeToHandle`, an action's `extraData`): a field TheHive sends without being asked is dropped whatever the model lists.
- **Page rows include `content` and `category` by default.** A page search returned only `_id`, `title` and `_createdAt`, so an agent found the page it wanted
  and could not read it unless it knew to ask for `extra-columns: ["content"]`, which no resource said. The search-entities reference already listed `category`
  as a default; now it is one. This also applies to the `pages` additional query on cases and alerts, whose rows grow by the pages' content.
- **Open WebUI replaces LibreChat as the web chat example.** The LibreChat stack pinned a v0.2.3 image, set no `THEHIVE_URL` (so a current server rejects every
  request), and needed a custom LibreChat image that nothing publishes. [`docker-compose.open-webui.yml`](deployment/examples/docker-compose.open-webui.yml)
  runs stock Open WebUI with TheHiveMCP registered as its MCP tool server; the guide is [docs/how-to/open-webui.md](docs/how-to/open-webui.md).

### Security

- **Built with Go 1.26.10, for ten standard-library vulnerabilities the server's code reaches.** Seven are in `net/http`, one in `crypto/tls`, one in
  `mime/multipart` (GO-2026-6603, -6605, -6607, -6608, -6610, -6611, -6612, -6613, -6617) and one in `crypto/x509` (GO-2026-6809). The vulncheck gate found the
  first nine in 1.26.8 on 2026-10-09, and the last in 1.26.9 on 2026-10-21. The binary and the image are built with 1.26.10; the source is unchanged.
  GO-2026-6809 (CVE-2026-111943) is the eMudhra (emSign) incident of 2026-10-20: the keys of eight intermediate CAs were compromised, so anyone could issue a
  certificate for the TheHive host and read the API key on a connection they intercept. The image's Debian trust store still reaches those intermediates through
  a Sectigo cross-sign, and Go checks no revocation; 1.26.10 refuses them in `crypto/x509` itself.

### Fixed

- **Responders work on TheHive 5.8 and later.** TheHive 5.8 added `extraData` to responder actions, and the TheHive client rejected any field it did not know,
  so every `run-responder` and `get-action-status` answered an error (`json: unknown field "extraData"`) although TheHive had created the action and the
  responder ran. A model told the run failed ran the responder again. The client (thehive4go) now ignores fields it does not know in every answer, which also
  covers what 5.8 and 6.0 added to tasks, tags, observable types and case statuses (#250).
- **A run TheHive answered 2xx to is never reported as a failure.** When TheHive answers 2xx to `run-analyzer` or `run-responder` but its answer cannot be read,
  or is empty, the error now says TheHive accepted the request, gives the HTTP status and the created job or action ID when the answer names one, and tells the
  model not to run it again and how to read it. Before, it said `failed to execute responder`, the cue to retry. A 2xx that is not JSON at all (a gateway's
  login page) says the run may not have happened and to check before running it again. A timeout or a 5xx still reads as a failure, although TheHive may have
  created the run (#252).
- **`hive://metadata/entities/custom-fields` works for organisations with custom fields.** It failed on every TheHive version as soon as one field had no
  options (`"options": []`) or integer options, which most organisations have (`data matches more than one schema in oneOf(OutputCustomFieldOptions)`), and its
  error carried the raw HTTP response, headers and Go pointers included. It lists the fields, and a failure now gives TheHive's status and message.
- **`search-entities` returns the columns asked for, on TheHive 5.8 too.** Fields the server's model of an entity does not know, such as a task's `timeToHandle`
  or an action's `extraData` on 5.8, came back on every row whatever the columns. They now come back only when named in `extra-columns`.
- **`:latest` moves only with a release.** A manual run of the release workflow from a branch pushed its snapshot image under `:latest` too, and created a
  GitHub release named after the branch. `:latest` is now pushed for a `vX.Y.Z` tag only, and a branch run publishes its `SNAPSHOT-<date>` image alone.
- **The image's build context leaves local files out.** `.dockerignore` listed duplicate and missing entries and let `.git`, the harness cache, `build/` and the
  worktrees into the context that `deployment/Dockerfile` copies whole. It now excludes them, along with local configuration, build outputs and the folders the
  image does not build from.
- **The filter guide the model reads matches the search tool.** `hive://docs/overview/filter-dsl` called the severity scale fixed, said `extra-columns` chose
  the columns, and left out paging. A model trusting it assumed `3` meant High in organisations that renamed the levels, re-listed default columns, and could
  not page past the first `limit` rows. It now says the scale is a per-organisation default, that extra columns add to the defaults, and how `offset`, `hasMore`
  and `nextOffset` page, within `limit` ≤ 1000 and `offset + limit` ≤ 10000.
- **The install and deployment guides work as written.** The binary downloads pointed at `releases/latest/download/thehivemcp-<target>`, a file no release
  publishes: they now fetch and extract the `thehivemcp-<version>-<target>.tar.gz` archive. The README named Claude Desktop's config file wrongly, the
  PowerShell check in the Windows guide never reached the binary, and the Claude Code guide had the scope precedence backwards. The permissions reference now
  lists the `case-template` and `page` entity types and the `apply-template` operation, which an `entity_permissions` block that omits them denies.

## [1.2.0] - 2026-10-06

**Upgrading from 1.1.x:** four changes can turn a working setup into an error. Each is described in full below.

- A permissions file with an unknown key (a typo such as `entity_permission:`) no longer starts the server. Fix the key the error names.
- A `pattern` filter or sort on `platforms` is refused, and `platforms` is no longer a default `pattern` column. Ask for it with `extra-columns: ["platforms"]`.
- An explicit `null` for a list parameter is refused by a gateway that validates arguments against the schema (LiteLLM). Omit the parameter instead.
- An `Authorization` header in another scheme than Bearer is no longer read as a TheHive key. Send the key as `Bearer` or in `X-TheHive-Api-Key`.

### Added

- **`execute-automation` can wait for analyzer jobs: operation `wait-for-jobs`.** It takes up to 20 `job-ids` and blocks until every one is final or
  `timeout-seconds` passes (default 45, at most 120), then returns each job with its verdict (the report's `success`, `summary` and `errorMessage`), plus
  `pendingJobIds` for any still running; `get-job-status` returns a job's full report. Real analyzers take minutes, and the only way to learn they had finished
  was to call `get-job-status` again: one model turn, and one resend of the whole conversation, per poll. Each job has its own outcome: one outside the
  permitted scope, unknown to TheHive, or unanswered in time is listed in `jobErrors` without costing the others their verdicts, and the timeout bounds every
  call to TheHive. Verdicts stay fenced as untrusted data.

### Changed

- **The warning for a date field that holds no timestamp no longer logs the field's value.** It logged the value, which can be entity data and so untrusted, and
  whenever the server returned an output schema it wrote out each date property's whole schema object. It now names the field and the value's type.
- **The MITRE ATT&CK catalog is no longer fenced as untrusted data.** A `search-entities` query on `entity-type: pattern` used to return its technique name and
  platform list wrapped in `[UNTRUSTED_DATA]` tags, although a pattern is reference data imported by an administrator, not case material — the trust boundary
  already treats the catalog as trusted, which is why `patternId`, `tactic` and `tactics` were exempt. The fence classifies by field _name_, and a pattern's
  fields are called `name`, `description`, `url` and `platforms`, which mean free user text on every other entity, so they could not simply join the allowlist;
  patterns are now exempted as a whole entity instead. A procedure is not exempt: only its catalog references (`patternId`, the newly trusted `patternName`,
  `tactic`) pass unfenced, while its analyst-written `description` stays wrapped. Boundary markers found inside a catalog value are still neutralized.
- **`platforms` is no longer a default `pattern` column.** A pattern row now returns `_id`, `patternId`, `name` and `tactics`; the platform list is four to six
  entries on most techniques and widened every catalog page for a detail few queries ask about. `extra-columns: ["platforms"]` brings it back.
- **A `pattern` filter on `platforms` is now rejected.** TheHive does not index the field, so it accepted such a filter and returned zero rows with no error,
  which a model reported as "no technique targets that platform". The search now fails fast with an error pointing at `extra-columns: ["platforms"]`, and the
  pattern fact sheet no longer lists platform among the filterable fields.
- **List parameters are advertised as a plain `array`.** `entity-ids` (`manage-entities`), and `extra-columns`, `extra-data` and `additional-queries`
  (`search-entities`) used to be advertised as `["null", "array"]`, a union the schema inference adds to every slice field. For an input it means nothing (an
  omitted parameter and a `null` one decode alike), but models read it as "not quite an array": one sent `extra-columns` as the JSON string
  `'["description", "platforms"]'`, and a gateway validating arguments against the schema (LiteLLM) rejected every call. No tool input schema offers `null` any
  more, at any depth. As a consequence, an explicit `null` for one of these parameters is now refused by such a gateway (`None is not of type 'array'`); omit
  the parameter instead.

### Fixed

- **Four answers no longer mislead a model.** All found in the 1.2.0 test run:
  - **An error's cause names TheHive's answer, not the client's decoding error.** When TheHive's error body held a field the generated client does not declare
    (a 400's `errors`), the cause read `json: unknown field "errors"`, which a model took for the reason. It now reads
    `TheHive answered 400 Bad Request: see its message in the API response`, where TheHive's message already was. A 2xx the client cannot decode keeps the
    client's error, which names the field.
  - **`wait-for-jobs` mentions retryable jobs only when there is one.** Its message said "a retryable one is also in pendingJobIds" after any failure, including
    a 404 that will never be retried.
  - **A capped `timeout-seconds` is said.** A value above 120 was lowered silently, so a model that asked for 500 s took the shorter wait for the whole one. The
    result message now starts with `timeout-seconds was capped at the maximum, 120.`
  - **`sort-by: platforms` on `pattern` is refused**, like a filter on it: TheHive does not index the field, accepted the sort and ordered nothing by it.

- **IDs are checked before they reach a URL path.** `get-job-status` and `wait-for-jobs` put the job ID into TheHive's job endpoint path unchecked: in the 1.2.0
  test run, `job-id: "../../user/current"` reached the HTTP layer and only the reverse proxy refused it. `entity-id` got this check after the RandoriSec 5.3
  audit; job IDs did not, and neither did `manage-entities`' `entity-ids` and `target-id` or the analyzer catalog's `dataType`. The generated client
  percent-encodes path values, which left a dot segment (`..`), an empty ID, and a proxy that decodes `%2F` before routing. Job IDs are now held to TheHive's ID
  shape (`~123456` or bare digits), like `entity-id`. `manage-entities` IDs and `dataType` refuse empty values, dot segments, slashes, backslashes and control
  characters; `apply-template`, which sends its IDs and template name in the request body, is left as it was, so a template name may still hold a slash. The
  tool description's job ID examples (`AWxyz123`, a pre-TheHive-5 shape) now show `~123456`.

- **Creating an observable that TheHive did not create is now an error.** An observable sent without `data` (a common slip is `value`) made TheHive answer 2xx
  with an empty list, and `manage-entities` reported "Observable created successfully" with an empty `result`. A model then believed the observable existed and
  built on it; one measured run spent its whole turn budget re-creating it. The tool now fails with a hint to set `entity-data.data`.

- **An observable create that TheHive completed in part is reported as such.** When some values already existed, TheHive created the others and answered 207
  with the created and the refused values; the client expected a plain list, so `manage-entities` reported `failed to create observable` with a JSON decoding
  error and a hint about permissions, although observables had been created. A model then retried, or told the analyst nothing was added. The create now
  succeeds with the observables made in `result` and TheHive's reason for each refused value in `failures`; when every value is refused, it fails and names the
  error type (`CreateError` for an observable that already exists). A partial create in a case is no longer retried on the alert endpoint. A create a case
  refused (an unknown `dataType`, no value) used to be reported with the alert endpoint's "Operation not permitted" and a permissions hint; it now carries the
  case's answer.

- **A TheHive that stops answering no longer hangs the tool call.** The connection to TheHive had no timeout, so a TheHive that accepted the connection and
  never answered held the call until the MCP client gave up — about four minutes in Claude Desktop — with no error, which read as a crashed server. Every
  request to TheHive is now bounded (2 minutes by default, set with `THEHIVE_REQUEST_TIMEOUT` or `--thehive-request-timeout`), and a request that runs out of
  time fails with an error saying so. Its hints warn that an abandoned create, update or delete may still have been applied, so the entity should be read back
  before retrying.

- **Responders on a task or an observable are reachable.** TheHive's responder endpoints know those entities as `case_task` and `case_artifact`, not the `task`
  and `observable` the tool and the responder catalog resource advertise, so the catalog and `run-responder` answered 404 for them. Both now translate, and a
  task log can be named `log` or `case_task_log`.

- **A search no longer rewrites the permission filters.** `search-entities` repaired filter keys and turned dates into timestamps in place, on maps that still
  belonged to the permission config, so the first search changed the config a session shows in `hive://config/permissions`. Over stdio, where one config serves
  every tool call of the session, two concurrent searches could write the same map, which stops a Go process. The search now works on a copy, as the scope
  checks already did.

- **`MCP_SERVER_ENDPOINT` is documented under its real name.** The README and the basic Docker Compose file used `MCP_ENDPOINT_PATH`, which the server never
  read, so a custom endpoint path set that way was silently ignored. The `--help` text also named `HIVEMIND_*` variables, and the HTTP startup error `MCP_URL`,
  none of which exist.
- **The basic Docker Compose file no longer reports the container as unhealthy.** Its healthcheck ran `wget` against `/health`, but the image is distroless,
  with no `wget`, and the server has no such route. The override is gone, so the image's own probe applies.
- **The arm64 Docker image is built on an arm64 base.** Both platforms of the image used to start from the same base, and `make docker-build` built a different
  set of platforms than a release. Each platform now has its own base, and `make docker-build` builds both, without running Go under an emulator.

### Security

- **A responder blocked by name can no longer run by its ID.** Analyzer and responder lists match a worker's ID or its name, and the shipped profiles use names
  (`blocked: ["DeleteCase_1_0"]`), but `run-analyzer` and `run-responder` only checked the identifier the caller sent. A responder blocked by name therefore ran
  when called by its ID, which is the identifier the tool description tells the model to use. The catalog also kept listing it. Both paths now check a worker
  under its ID and its name together. The fix also works the other way: an analyzer allowed by name is no longer refused when called by its ID. When the rules
  name specific workers, a run first reads the analyzer or responder catalog to learn the worker's other identifier, and a worker missing from the catalog is
  denied. Rules that name none (`"*"`, an empty block list) need no lookup and behave as before.
- **`run-responder` and `get-action-status` refuse a malformed `entity-type` or `entity-id`.** Both values go into the path of a Cortex endpoint, and nothing
  enforced the schema's enum at runtime. An entity ID must now be a TheHive ID (`~123456`, or its bare digits). The type may be given in any case, or as TheHive
  spells it (`case_task`, `case_artifact`).
- **A misspelt key in the permissions file now stops the server.** Unknown keys used to be dropped without a word, and what they named fell back to its most
  permissive default: `entity_permission:` in place of `entity_permissions:` allowed every operation on every entity, and `filter:` in place of `filters:`
  removed the filters. The file is now parsed strictly and checked at startup, over stdio and over HTTP, and the server refuses to start on an unknown key,
  naming it. Before, a file that failed to load only logged a warning, and every tool call was then denied with no reason given; that is still what happens when
  the file is edited into an invalid state while the HTTP server runs, since it reloads the file per request. **A file that contains such a typo no longer
  starts: fix the key it names.**
- **A client's own TheHive key is no longer replaced by the server's.** With `ALLOW_ENV_CREDENTIAL_FALLBACK=true`, a key sent in the `Authorization` header was
  overwritten by `THEHIVE_API_KEY`, so the client's calls ran under the service account: with its rights, and under its name in TheHive's audit trail. The
  server's key now stands in only when a request carries no key.
- **An `Authorization` header in another scheme than Bearer is no longer taken for a TheHive key.** The Basic credentials an authenticating proxy passes on were
  forwarded to TheHive as the key, so the proxy user's password reached TheHive and the call failed. Such a value is now ignored: a request carrying only that
  gets the env fallback when it is on, and is refused otherwise. When a request sends both headers, `X-TheHive-Api-Key` wins.
- **The HTTP server no longer waits indefinitely on a slow or huge request.** It served with no timeouts and read a request body of any size in full before
  checking credentials, so a client that could reach the port could hold a connection with headers or a body sent a byte at a time, or exhaust the server's
  memory with one huge body. Headers must now arrive within 10 s and a whole request within 30 s, and a body is capped at 4 MiB: one that declares more is
  refused with 413 before any of it is read, and a chunked one is cut off at the cap with a JSON-RPC parse error. Long tool calls and SSE streams are
  unaffected, since these limits stop applying once the request has been read. They do not cap how many connections or streams a client opens, which the
  authenticating proxy the deployment docs require must still do.

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

[Unreleased]: https://github.com/StrangeBee/TheHiveMCP/compare/v1.2.1...HEAD
[1.2.1]: https://github.com/StrangeBee/TheHiveMCP/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/StrangeBee/TheHiveMCP/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/StrangeBee/TheHiveMCP/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/StrangeBee/TheHiveMCP/compare/v0.3.4...v1.0.0
[0.3.4]: https://github.com/StrangeBee/TheHiveMCP/compare/v0.3.3...v0.3.4
