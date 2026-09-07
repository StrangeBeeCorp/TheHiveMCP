# ADR-0003 - Track MCP 2026-07-28 on mark3labs/mcp-go v1.0.0, rather than migrating to the official Go SDK

- Status: proposed
- Date: September 4, 2026
- Deciders: TheHiveMCP maintainers
- Related: DL-6920, [#170](https://github.com/StrangeBee/TheHiveMCP/issues/170),
  [MCP 2026-07-28 changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog)

## Context and problem statement

[MCP 2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28/changelog) is a breaking revision. It removes the `initialize` handshake, protocol
sessions and the `Mcp-Session-Id` header, the standalone HTTP `GET` stream, `ping`, `logging/setLevel` and server-initiated requests; it adds a mandatory
`server/discover`, required `Mcp-Method`/`Mcp-Name` headers, a `resultType` on every result, cache hints on list results, and Multi Round-Trip Requests (MRTR)
in place of mid-call server-to-client requests.

**Nothing is broken and nothing is urgent.** A handshake-based server keeps working against dual-era clients, which is what the tier-1 clients ship. The
question this ADR answers is which SDK we track, because everything else follows from it — see
[What is deliberately not being done now](#what-is-deliberately-not-being-done-now).

An earlier revision of this ADR recommended migrating to `modelcontextprotocol/go-sdk`, on the premise that `mark3labs/mcp-go` had no implementation of the
revision and no timeline for one. **That premise expired on September 2, 2026**, when `mcp-go` released
[v1.0.0](https://github.com/mark3labs/mcp-go/releases/tag/v1.0.0), whose headline change is
[PR #951, "support the 2026-07-28 specification"](https://github.com/mark3labs/mcp-go/pull/951). It claims SEP-2575 (stateless core, `server/discover`,
`subscriptions/listen`), SEP-2322 (MRTR, `resultType`), SEP-2243 (header routing), SEP-2549 (cacheable results), SEP-2567 (session removal), and the
`-32020`/`-32021`/`-32022` error renumbering.

That reverses the decision, so the comparison is restated against what is actually released today:

|                     | `mark3labs/mcp-go` (current)                              | `modelcontextprotocol/go-sdk`                                      |
| ------------------- | --------------------------------------------------------- | ------------------------------------------------------------------ |
| Version in use here | v0.43.1 → protocol **2025-06-18**                         | —                                                                  |
| Latest release      | **v1.0.0 (2026-09-02) → protocol 2026-07-28**             | **v1.7.0 (2026-07-28) → protocol 2026-07-28**                      |
| 2026-07-28 support  | Shipped, five weeks after the revision                    | Shipped on the revision's release day                              |
| Cost to reach it    | **A dependency bump: 3 Go files, +9/−5** (measured below) | A rewrite: ~650 symbol references across 37 files (measured below) |
| Maintenance         | Community                                                 | Reference implementation, maintained alongside the specification   |

Note that [issue #928](https://github.com/mark3labs/mcp-go/issues/928), cited in the earlier revision as evidence that support was absent and unowned, is still
open — but it has been overtaken by #951 and is no longer evidence of anything. An open issue is not a reliable proxy for a missing feature.

## Verification

Release notes are a claim, not a result. The claim was tested against this repository: `go get github.com/mark3labs/mcp-go@v1.0.0` on `main` (755a282), then
`go build`, `go vet`, the `-short` suite, a spike exercising the real server over both protocol eras, the full integration suite against a live TheHive, and
finally the shipped `cmd/server` binary driven by a plain HTTP client against a live TheHive. The last of these is what the rest of this section rests on: it is
the only measurement taken on the artefact we actually deploy.

### The bump costs three Go files

`go build ./...` surfaced two incompatibilities, neither introduced by the protocol work:

| Change                                                                                                               | Landed in | Fix                                  |
| -------------------------------------------------------------------------------------------------------------------- | --------- | ------------------------------------ |
| `server.OnAfterCallToolFunc`'s `result` parameter widened from `*mcp.CallToolResult` to `any`                        | v0.58.0   | Type-assert in `onAfterCallToolHook` |
| `mcp.ClientCapabilities.Sampling`/`.Elicitation` became typed `*mcp.SamplingCapability`/`*mcp.ElicitationCapability` | v0.58.0   | Replace two `&struct{}{}` literals   |

```text
 go.mod                                       | 12 ++++--------
 go.sum                                       | 25 ++++++++++++-------------
 internal/logging/mcp_logging.go              |  8 ++++++--
 internal/testutils/mcp_client.go             |  4 ++--
 internal/utils/elicitation_transport_test.go |  2 +-
```

Five files, but only the last three are hand-edited: `go.mod` and `go.sum` are regenerated by `go mod tidy`. The hand-written change is **+9/−5 across three Go
files**, which is the figure quoted elsewhere in this ADR.

After that, `go build ./...` and `go vet ./...` are clean and every `-short` package passes — `bootstrap`, `internal/permissions`, `internal/resources`, all
four tool packages, and `internal/utils`. No handler, no tool definition and no resource registration needed touching.

### The real server speaks 2026-07-28 with no code change

The bump alone, with `bootstrap.GetMCPServerAndRegisterTools()` exactly as it is written today, served over `server.NewStreamableHTTPServer`:

```text
=== RUN   TestSpike_ModernClientNegotiates20260728
    negotiated protocol version: "2026-07-28"
=== RUN   TestSpike_ServerDiscover
    supportedVersions=[2026-07-28 2025-11-25 2025-06-18 2025-03-26 2024-11-05]
=== RUN   TestSpike_ToolsAndResourcesOverModernPath
    tools over 2026-07-28: [execute-automation get-resource manage-entities search-entities]
    resources over 2026-07-28: 49
=== RUN   TestSpike_LegacyClientStillWorks
    legacy negotiated protocol version: "2025-11-25"
```

All four tools and all 49 resources list over the stateless path, `server/discover` answers, and a legacy-only client negotiates down on the _same_ endpoint.
Both eras are served concurrently, which the spec blesses and which `mcp-go` pins with its own conformance archive
(`server/testdata/conformance/legacy_unchanged.txtar`, asserting the exact bytes a pre-2026 client receives: no `resultType`, no `ttlMs`, no `_meta`).

### One thing genuinely breaks: the write-confirmation gate

`internal/utils/elicitation_transport.go` confirms modifying operations by calling `MCPServer.RequestElicitation` from inside an `http.RoundTripper`, mid-call.
That is precisely the server-initiated request the revision removes. Reproduced with a server whose tool handler does what `RoundTrip` does:

```text
=== RUN   TestSpike_RoundTripperGate_ModernClient
    err=internal error: "elicitation/create": server-initiated requests are not supported in protocol
        version 2026-07-28 or later: return an InputRequests map from the handler instead
        (multi round-trip requests, SEP-2322)
=== RUN   TestSpike_RoundTripperGate_LegacyClient
    legacy: err=<nil> result=...Text:written... elicitations=1
```

And a modern session does report the client's capability, so the gate takes its confirming branch rather than its permissive one:

```text
=== RUN   TestSpike_ModernSessionClientCapabilities
    modern: haveSession=true elicitation advertised=true
```

Consequently, over Streamable HTTP, a modern connection from an elicitation-capable client **fails every modifying operation**. (The qualifier matters: over the
in-process transport the same call succeeds — see [the next section but one](#the-breakage-is-transport-specific-and-the-integration-suite-is-blind-to-it).) The
error is not `server.ErrElicitationNotSupported`, so it does not even take the deliberate fail-closed branch (DL-6005) — it falls through to the generic
`elicitation request failed:` wrapper.

This is the _only_ thing that breaks. Everything else about the modern path works untouched, which is what makes the resolution below possible: **the
confirmation layer is removed rather than ported to MRTR** (see [Decision](#decision) and [#170](https://github.com/StrangeBee/TheHiveMCP/issues/170)).

### End to end on the shipped binary, both eras

The spikes above construct servers in-process. This one does not: `cmd/server` was built against v1.0.0 and run as it ships — HTTP transport, its own auth
middleware and URL allowlist — against TheHive 5.6.3 (Cassandra + Elasticsearch), with a real organisation, an org-admin user and a real API key. The client was
`curl`, so nothing in the MCP client library could paper over a protocol mistake.

Legacy (`2025-11-25`), the full handshake:

```text
initialize      → "2025-11-25", session mcp-session-edeb4fc2…
tools/list      → execute-automation, get-resource, manage-entities, search-entities
resources/list  → 49 resources
search-entities → {"count":0,"entityType":"case","results":[]}
manage-entities → "Case created successfully"  _id ~8614056
```

Modern (`2026-07-28`), stateless, no session:

```text
server/discover → supportedVersions [2026-07-28 2025-11-25 2025-06-18 2025-03-26 2024-11-05], resultType "complete"
tools/list      → all 4 tools, resultType "complete"
tools/call      → "Case created successfully"  _id ~8687784   (client advertising no elicitation)
```

Both cases were then read back out of TheHive's own API rather than trusted from the MCP response:

```text
~8614056  E2E real-binary case
~8687784  modern-no-elicit
```

The binary also enforces the new header routing: without `Mcp-Method` a modern request is rejected with `-32020 header mismatch`, which is SEP-2243 behaving as
specified.

### The breakage is transport-specific, and the integration suite is blind to it

The failure above is **not** universal to modern connections — it depends on the transport, which matters because our test harness and our deployment do not use
the same one.

Over the **in-process** transport, a modern session negotiates `2026-07-28`, the gate fires, and the write succeeds:

```text
NEGOTIATED = "2026-07-28"  (modern=true)
WRITE isError=false → "Case created successfully" _id ~3903640
ELICITATIONS SEEN = 1
```

Over **Streamable HTTP** — the production transport, `bootstrap/http.go` — the same server, same client, same modern protocol:

```text
NEGOTIATED = "2026-07-28"  (modern=true)
WRITE → elicitation request failed: "elicitation/create": server-initiated requests are not
        supported in protocol version 2026-07-28 or later
ELICITATIONS SEEN = 0
```

`testutils.GetMCPTestClientWithPermissions` builds its client on `transport.NewInProcessTransportWithOptions`, so **the entire integration suite runs on the one
path where this works**. It was run both ways to confirm the point: 348 tests, zero failures, on v0.43.1 and on v1.0.0 alike. A green suite is therefore not
evidence that modern clients can write, and would stay green through the whole regression window.

Note also that `LATEST_PROTOCOL_VERSION` silently changed meaning: the test client at `internal/testutils/mcp_client.go` asks for it by name, so the same source
line requests `2025-06-18` on v0.43.1 and `2026-07-28` on v1.0.0. The suite quietly moved to the modern protocol without anybody choosing that.

### The SDK's documented escape hatch does not work

`mcp-go` offers `server.WithLegacyServerInitiatedRequests()` to keep server-initiated requests on modern connections. It is not a usable bridge: over Streamable
HTTP with a modern client it **deadlocks** rather than erroring, blocking until the context is cancelled.

```text
goroutine 17 [select]:
  mcp-go@v1.0.0/server/streamable_http.go:1982  (*streamableHttpSession).RequestElicitation
  mcp-go@v1.0.0/server/elicitation.go:35        (*MCPServer).RequestElicitation
  ...                                            handleToolCall → HandleMessage → handlePost
```

The request is queued onto `elicitationRequestChan`, which nothing drains for a stateless request, and the following `select` waits only on the response channel
and `ctx.Done()`. This should be reported upstream. Do not plan around the opt-out.

## Decision

**Stay on `github.com/mark3labs/mcp-go` and upgrade to v1.0.0.** Do not migrate to `modelcontextprotocol/go-sdk`.

The reasoning is entirely about cost, because both SDKs now implement the same revision and both bridge the two eras for us. One route is a dependency bump
touching three Go files that leaves every handler, tool and resource untouched and is verified green; the other rewrites roughly 650 SDK references across 37
files and cannot be judged complete until the container suite passes. When two options reach the same protocol, the cheap one wins, and no argument from the
earlier revision survives the release of v1.0.0 — its case rested on `mcp-go` not having the revision.

**Remove the elicitation confirmation layer as part of the bump; do not port it to MRTR.** Recorded as
[#170](https://github.com/StrangeBee/TheHiveMCP/issues/170), to be reconsidered if MRTR earns the client adoption elicitation never did.

This is a product decision, and it rests on adoption rather than on protocol mechanics. Elicitation has been available for a long time and remains
near-unsupported: our own README records that GitHub Copilot implements it and that most clients, Claude Desktop included, do not. Real-world use has been close
to nil, and the feedback we have is negative. There is no reason to expect MRTR to land differently in the short term.

Crucially, it is not the security boundary. Authorisation is the caller's TheHive API key and the permissions configuration; confirmation is one optional layer
above both, and clients that do not advertise the capability already write unconfirmed today. Removing it does not widen what any principal may do — it removes
a prompt most clients never displayed. Meanwhile porting it is the single most expensive and most security-sensitive piece of the upgrade: MRTR inverts when the
server must know confirmation is needed, and `requestState` travels through the client, so it must be signed or the confirmation is forgeable and therefore
decorative. That is a design worth doing carefully for a feature people use, and hard to justify for one they do not.

Sequence the work in three changes:

1. **The bump, with elicitation removed, fenced to legacy** (v0.43.1 → v1.0.0, the three files above, minus the confirmation layer, plus
   `server.WithStreamableHTTPProtocolVersions(mcp.LegacyProtocolVersions()...)` on the transport). Removing the layer must include dropping
   `server.WithElicitation()`, so the capability is no longer advertised — advertising what we do not implement is the bug fixed in #168.
2. **Streamable HTTP coverage on both eras**, which does not exist today and is why this breakage was invisible.
3. **Lift the fence**, as its own reviewable, revertible commit.

**Why keep the fence at all, once elicitation is gone?** With the layer removed there is no known modern-path regression — verified: a modern client advertising
no elicitation creates a case successfully against a live TheHive. The fence is therefore no longer protection against a known fault, and step 1 could
reasonably ship unfenced. It is retained for one narrower reason: the coverage in step 2 does not exist yet, so nothing would tell us if the modern path
regressed. Fencing keeps us on the code path `mcp-go` pins byte-for-byte with its `legacy_unchanged.txtar` conformance archive — the mature half of a library
whose modern half is days old and already known to contain the deadlock documented above — until we can actually observe the other one. It costs one line, and
modern clients simply negotiate down. Shipping step 1 unfenced is a defensible alternative if the coverage lands alongside it.

## Consequences

### Deployments using an elicitation-capable client lose their confirmation prompts

This is the real user-visible cost of the decision, and it must not be discovered in production. For the minority running a client that implements elicitation —
GitHub Copilot, in practice — **every modifying operation** stops prompting, not just the obvious three. `ElicitationTransport` gates on the HTTP verb, not on
the tool: it confirms every `POST`, `PATCH` and `DELETE` to TheHive except the read-only `/api/v1/query` endpoint. So the affected set is all seven
`manage-entities` operations — `create`, `update`, `delete`, `comment`, `promote`, `merge`, `apply-template` — plus the two `execute-automation` writes,
`run-analyzer` and `run-responder`.

That breadth is worth stating plainly for release planning: anything that reaches TheHive with a modifying verb loses its prompt, including operations added
after this change, since the gate that would have caught them no longer exists.

The DL-6005 fail-closed behaviour goes with it: there is no longer an advertised prompt that can fail to complete, so there is nothing to fail closed on.

Anyone who relied on the prompt should restrict the permissions configuration or scope the TheHive API key, which were always the actual controls. This needs a
clear CHANGELOG entry and a release note, plus removal of the README and `docs/reference/tools/manage-entities.md` sections that promise the behaviour.

### The fence buys observability, not correctness

Once elicitation is gone there is no known modern-path fault to fence against. What the fence still buys is time: until the Streamable HTTP coverage in step 2
exists, we have no instrument that would notice a modern-path regression, and the in-process suite will keep reporting green regardless. Fencing holds us on the
conformance-pinned legacy path until that instrument exists.

What is deferred is reach, not correctness: until step 3, TheHiveMCP does not serve `2026-07-28` and does not offer `server/discover`. That is the same posture
as today on v0.43.1 — with 20 releases of fixes and none of the drift.

### We are choosing a community SDK over the reference implementation, knowingly

This is the one durable argument for `go-sdk`, and it is not answered by v1.0.0 — it is only outweighed. `mcp-go` reached this revision five weeks after
publication, and its own release notes record deliberate gaps (authorization hardening untouched; tasks not moved behind the `io.modelcontextprotocol/tasks`
extension; conformance vectors hand-written from the spec rather than pulled from the cross-SDK corpus). The deadlock documented above is a second data point
about depth of coverage. Treat sustained lag on a future revision, not this one, as the trigger to reopen the question — and reopen it as a new ADR, since the
reference measurement below will have gone stale by then.

### What the rejected migration would have cost

Recorded so the scale is not re-derived if the question reopens. Counted on `main` (755a282) as qualified references to the mcp-go packages a file imports —
`mcp.`, `server.`, `client.`, `transport.` followed by an exported identifier — which gives **646 references across 37 files** (23 non-test, 14 test):

| Package              | Refs | Files | Character of the work                                                                     |
| -------------------- | ---- | ----- | ----------------------------------------------------------------------------------------- |
| `internal/resources` | 329  | 4     | Mechanical and highly repetitive — 49 near-identical `NewResource` registrations          |
| `internal/tools`     | 100  | 18    | Tool definitions and registration; handler business logic is mostly portable              |
| `bootstrap`          | 69   | 8     | Real design work: server construction, transports, hooks → middleware, auth context funcs |
| `internal/testutils` | 55   | 2     | Test client and in-process transport                                                      |
| `internal/logging`   | 44   | 1     | Hooks become receiving middleware                                                         |
| `internal/utils`     | 33   | 3     | Mostly the elicitation transport and its test, which this decision deletes anyway         |
| `internal/auth`      | 16   | 1     | Middleware signature change                                                               |
| **Total**            | 646  | 37    |                                                                                           |

An earlier revision of this ADR recorded 685 references across 36 files, with per-package rows that summed to 674 across 35 — the totals and the rows disagreed,
and neither stated what was being counted. The table above replaces both: it states its counting rule, and it adds up. Treat it as an order of magnitude rather
than a precise figure, since it moves with every commit — the point it supports is that the rewrite is two orders of magnitude larger than the bump, and that
conclusion is insensitive to the exact number.

### It must still be validated against a live TheHive — and the suite is the wrong instrument for the modern path

The `-short` suite passing is necessary, not sufficient. The integration suite gates on `testutils.StartTheHiveContainer`; neither step is done until it passes.
This is much less of a risk for a bump than it would have been for a rewrite, but the bump does change result marshalling on the legacy path, so it is not
skippable. It has been run: 348 tests, zero failures, on v0.43.1 and v1.0.0 alike.

That result must not be over-read. Because the suite is in-process, it exercises the legacy semantics of the confirmation gate no matter which protocol version
it negotiates, so it certifies the fenced configuration and nothing beyond it. Removing the gate narrows the gap — with no server-initiated requests left, the
two transports stop diverging on the one behaviour that separated them — but it does not close it: the suite still never exercises Streamable HTTP. Lifting the
fence cannot be signed off by the suite as it stands; it needs the coverage named below, and an end-to-end run against the real binary of the kind recorded in
[Verification](#verification).

### `hive://config/*` resource output may shift on modern connections

Resource content is ours, but envelope shapes (`resultType`, cache hints, `_meta`) come from the SDK, and only on modern requests — the conformance archive
guarantees the legacy envelope is byte-identical. Worth diffing a captured `resources/read` response on both eras.

### Deliberately deferred

`ttlMs` and `cacheScope` values are a judgement call about TheHive data freshness, not a mechanical port — the static catalog tolerates a long TTL, live entity
resources do not. `mcp-go` exposes them via `server.WithCacheHints`/`WithMethodCacheHints`. Land conservative values and tune separately.

## Alternatives considered

**Migrate to `modelcontextprotocol/go-sdk`.** This was the earlier recommendation, and the spike behind it stands: `go-sdk` v1.7.0 negotiates 2026-07-28, and
its `serverMultiRoundTripMiddleware` serves older clients by calling them directly and re-invoking the handler, so one handler implementation covers both eras.
Rejected only on cost, now that `mcp-go` v1.0.0 offers the same bidirectional MRTR bridge — `mcp-go`'s equivalent is `server/mrtr.go`, reached through
`NewInputRequestBuilder` and `server.ElicitationResponse`. Paying for a ~650-reference rewrite to obtain a protocol we can have for a three-file bump needs a
justification that the maintenance argument alone does not carry.

**Bump to v1.0.0 but keep serving legacy only.** Adopted as step 1, not rejected — this is the fence. It is a waypoint rather than a destination: held
indefinitely it would take the upgrade's cost without its headline benefit, since the point of reaching v1.0.0 is eventually to serve `2026-07-28`. Its value is
that it banks the 20 releases of fixes immediately, at zero protocol risk, while the Streamable HTTP coverage is written.

**Stay on v0.43.1.** Rejected. It was defensible while the alternative was a rewrite; it is not defensible against a three-file bump. It also leaves us two
revisions behind (2025-11-25 and 2026-07-28), and forgoes the 20 intervening releases of unrelated fixes.

**Run both SDKs side by side.** Rejected: two servers to keep consistent, and the permission and confirmation layers would need duplicating — precisely the code
where divergence is most dangerous.

## What is deliberately not being done now

Three changes were prototyped alongside this investigation and **rejected as premature**, recorded here so they are not proposed again as separate work:

- **Accepting `405` in the container healthcheck.** Correct for a `2026-07-28` server, but this server cannot emit `405` until it serves the modern path, so the
  change has no effect until then and belongs with the work that enables it.
- **Consolidating the logging correlation keys.** The `session_id` placeholder only becomes noise once protocol sessions are gone, and it stays meaningful for
  as long as legacy clients are served — which, per the dual-era design, is indefinitely. Revisit only if the legacy path is retired.
- **Moving write confirmation out of the `http.RoundTripper`.** Overtaken: the confirmation layer is being deleted rather than relocated, so the refactor has no
  subject. If [#170](https://github.com/StrangeBee/TheHiveMCP/issues/170) is ever taken up, this becomes live again — and MRTR semantics, not tidiness, should
  determine its shape.

### Elicitation was an optional layer, by design — which is what makes removing it defensible

When a client does not advertise `elicitation`, modifying requests already proceed unconfirmed. **That is intentional, not a defect.** Authorisation is the
permissions configuration and the caller's TheHive API key; elicitation is one optional convenience layer above both, and clients manage approval in their own
ways. `TestRoundTrip_NoCapabilityProceeds` records the reasoning.

That property is precisely why the layer can be dropped rather than ported. Removing it does not change what any principal is permitted to do; it generalises
the unconfirmed path that most clients were already on. What would have been a genuine security-posture change is the opposite move — turning a missing
capability into a refusal — and that is not what is happening here.

One honest note on what is lost. Before this decision, the modern-client failure was an _accidental_ refusal: an SDK error that is not
`ErrElicitationNotSupported`, not a deliberate policy. Removing the layer resolves the accident by deleting the policy along with it, rather than by expressing
the policy correctly on both eras. That is the trade [#170](https://github.com/StrangeBee/TheHiveMCP/issues/170) exists to revisit.

## Work, gated on accepting this ADR

1. Bump `mcp-go` to v1.0.0: type-assert in `onAfterCallToolHook`, replace the two `&struct{}{}` capability literals, `go mod tidy`. Add
   `server.WithStreamableHTTPProtocolVersions(mcp.LegacyProtocolVersions()...)` in `StartHTTPServer`, so the bump lands fenced. Validate against a live TheHive.
2. Remove the confirmation layer in the same change: delete `internal/utils/elicitation_transport.go` and its test, stop wrapping the HTTP client in
   `bootstrap/common.go`, and drop `server.WithElicitation()` from both `bootstrap/server.go` and `bootstrap/inprocess.go` so the capability is no longer
   advertised. Drop the now-unused elicitation and sampling handlers from `internal/testutils/mcp_client.go`. Update the README section,
   `docs/reference/tools/manage-entities.md`, and the CHANGELOG — the behaviour change is user-visible and must be announced, not discovered.
3. Add coverage asserting both a modern and a legacy client work **over Streamable HTTP**, not only in-process — the in-process-only suite is what hid this
   breakage. Assert the modern write path explicitly, so removing the fence cannot pass silently. The spike tests above are the starting point.
4. Remove the fence, as the last step and on its own commit, so the change that begins serving `2026-07-28` is reviewable in isolation and revertible by itself.
5. `ttlMs`/`cacheScope` via `WithCacheHints`, resource-not-found `-32002` → `-32602`. Deterministic `tools/list` ordering is already satisfied incidentally.
6. Replace the hand-written `initialize` snippets in `docs/how-to/` with `server/discover`.
7. Report the `WithLegacyServerInitiatedRequests()` deadlock upstream.
