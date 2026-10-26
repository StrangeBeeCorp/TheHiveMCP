---
name: develop-feature
description:
  How to build a change in TheHiveMCP, the Go MCP server for TheHive. Covers where tools, resources and permissions live, the schema and trust-boundary rules
  model-facing output must keep, testing on TheHive 5.5, 5.8 and 6.0, and what a PR must update.
---

# Develop a feature in TheHiveMCP

## Where things live

| Area                 | Path                                                                          | Notes                                                                                                                                                                                     |
| -------------------- | ----------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Server wiring        | `bootstrap/server.go`                                                         | `RegisterToolsToMCPServer` registers every tool and resource                                                                                                                              |
| Transports, auth     | `bootstrap/{stdio,http,inprocess,allowlist}.go`                               | `AuthMiddleware` is in `inprocess.go`                                                                                                                                                     |
| One tool             | `internal/tools/<tool>/`                                                      | `definition.go` (schema, annotation), `models.go` (params, results, the `*ToolDescription` text the model reads; manage keeps its results in `results.go`), `handler.go`, `validation.go` |
| Shared tool code     | `internal/tools/{tool,middlewares,schema,union_schema,annotations,errors}.go` | `WithValidation` runs `ValidateParams` then `ValidatePermissions` before `Handle`                                                                                                         |
| Resources            | `internal/resources/`                                                         | Embedded `facts/*.txt`, `rules/*.txt`, `schemas/<entity>/{Create,Output,Update}*.json`, `internal/resources/docs/filter-dsl.md`. All hand-written, all read by the model                  |
| Permissions          | `internal/permissions/`                                                       | Default profile `embedded/default_permissions.yaml` (read-only). Examples in `docs/reference/permissions-examples/`                                                                       |
| Config keys          | `internal/types/constants.go`, `options.go`                                   | Flags override env vars                                                                                                                                                                   |
| Wrapping and parsing | `internal/utils/parsers.go`                                                   | The `[UNTRUSTED_DATA]` boundary; `dates.go` and `fields.go` hold the date conversion and the JSON helpers it walks with                                                                   |

## Workflow

1. **Classify the change.** It is _model-facing_ if it touches a tool, a tool or parameter description, a schema, a resource text, an error message, or
   auth/authz. Model-facing changes must follow the rules below, and the PR must tick the "can affect model-facing behavior" box in
   `.github/PULL_REQUEST_TEMPLATE.md`. That box makes the next release re-run the accuracy and security suites (`RELEASING.md`).
2. **Missing TheHive API field or endpoint?** Fix it in thehive4go, not here. Regenerate it there, tag a release, then bump
   `github.com/StrangeBeeCorp/thehive4go` in `go.mod`.
3. **Implement test first.** Run the `implement-task` workflow (`.claude/workflows/implement-task.js`, args `{ticket, task, items}`): the Tester
   (`.claude/agents/tester.md`) writes failing tests from the task's "Done when", the Coder (`.claude/agents/coder.md`) makes them pass, and the Tester attacks
   until it finds nothing more. The hooks keep each to its files, and every stop runs `make check-fast`. Review what they did, then commit. Read the reference
   that matches the change first:
   - adding a tool, an operation or a parameter → `references/tool-anatomy.md`
   - a new response field, an error that quotes entity data, or any edit to `internal/utils/parsers.go`, `dates.go` or `fields.go` →
     `references/untrusted-data.md`
   - any test → `references/testing.md`
4. **Test.** Use `make check-fast` while you iterate: it runs the gates your branch's changed paths select (`docs/reference/gates.md`); lint and the unit tests
   cover the uncommitted files and the last commit's, and `make pr` and CI run them on everything. Run `make test-integration RUN=<regexp>` for anything that
   calls TheHive. It must pass on 5.5, 5.8 and 6.0: CI runs all three.
5. **Update the docs in the same PR:**
   - a tool's behaviour or parameters → `docs/reference/tools/<tool>.md`, plus its summary under "MCP Tools" in `README.md`
   - a permission key → `docs/reference/permissions.md` and the example profiles
   - anything a user can see → an entry under `## [Unreleased]` in `CHANGELOG.md` (Keep a Changelog: Added / Changed / Fixed). Existing entries say what changed
     and why the old behaviour misled a model or a user. Follow that style.
   - anything the model reads (a tool, a description, a schema, a resource text) → `make surface-propose`, and review the diff of `baselines/surface/` it
     writes. The surface gate fails until the snapshot matches, the changelog gate until `CHANGELOG.md` has its entry, and the tokens gate when
     `gates/budgets.yaml` is exceeded.
6. **Measure a model-facing change** before review, from a checkout of StrangeBee/TheHiveMCPEval:
   `uv run eval-cli eval run mcp-tools --local <this checkout> --model <label> --judge glm-5.3-judge`. `<label>` is a row of that repo's
   `configs/providers.yaml`. Without `--judge`, the model grades itself. The run is tagged `MCP local`, so it can never be published as release evidence.
7. **Gate and ship.** Run `make all` before the review. Once the review is ready for HEAD, `make pr` runs the pr gates at that exact commit, records the
   evidence (`bin/harness evidence --markdown` prints it for the pull request; add `--base origin/<its base>` for a stacked one) and pushes; from an agent's
   session it refuses without both, and `make pr` is the only way it pushes. After a rebase, the review that read the branch still counts: `make pr FORCE=1`
   lists the commits the rebase changed and those added since, for whoever merges. Agents never merge: a person does. CI runs the gates of `gates/registry.yaml`
   again on the pull request.

## Rules for model-facing code

Each rule below is an item of `spec/items.yaml`, proven by the tests and gates it names (a test that proves one carries `// spec: <ID>` on the line above it).
The analyzers gate (`internal/analyzers/`) checks the untrusted-data boundary, the hints, the cut flags and `testutils.Parallel`, and the canaries of
`gates/canaries/` show each check catching its breach when they run (`bin/harness canaries`, the canaries gate of the nightly cadence).

- **A silent success is a bug.** When TheHive answers 2xx but did nothing, the tool must fail and give a hint. Examples: a create that returns an empty list, or
  a filter on a field TheHive does not index (it returns zero rows and no error). The recent `### Fixed` entries in `CHANGELOG.md` are all of this kind. Return
  `tools.NewToolError(...).Hint(...)`. Error messages and hints are written by us, so they reach the model unwrapped: never put entity text in them.
- **Say what was cut.** Truncated or paged output must say so. `search-entities` returns `hasMore` and `nextOffset`, and the automation catalogs return
  `truncated`. A full page must never look like the complete result.
- **Support TheHive 5.5 to 6.0.** A feature that exists only on a later version must be gated by version, or left out on older ones without breaking anything
  (as the `filterable` annotation is), and its docs must say since which one (for example "5.6+").
- **Keep the filter doc's layout.** `internal/resources/docs/filter-dsl.md` is excluded from the markdown formatters because its compact JSON lines are what the
  model sees. Do not reflow it.
- **Keep text and code in sync.** Tool descriptions and `facts/*.txt` must list only fields that are actually filterable, and only operations that actually
  exist. A schema claim the handler does not honour is a bug. `internal/resources/filterable.go` publishes, per field, whether TheHive can filter on it. Update
  it when you change what a filter accepts.
