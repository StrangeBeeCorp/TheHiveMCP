# ADR-0003 - Track MCP 2026-07-28 on mark3labs/mcp-go v1.0.0, rather than migrating to the official Go SDK

- Status: proposed
- Date: September 3, 2026
- Deciders: TheHiveMCP maintainers
- Related: DL-6920, [MCP 2026-07-28 changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog)

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

|                     | `mark3labs/mcp-go` (current)                             | `modelcontextprotocol/go-sdk`                                     |
| ------------------- | -------------------------------------------------------- | ----------------------------------------------------------------- |
| Version in use here | v0.43.1 → protocol **2025-06-18**                        | —                                                                 |
| Latest release      | **v1.0.0 (2026-09-02) → protocol 2026-07-28**            | **v1.7.0 (2026-07-28) → protocol 2026-07-28**                     |
| 2026-07-28 support  | Shipped, five weeks after the revision                   | Shipped on the revision's release day                             |
| Cost to reach it    | **A dependency bump: 3 files, 8 lines** (measured below) | A rewrite: 685 symbol references across 36 files (measured below) |
| Maintenance         | Community                                                | Reference implementation, maintained alongside the specification  |

Note that [issue #928](https://github.com/mark3labs/mcp-go/issues/928), cited in the earlier revision as evidence that support was absent and unowned, is still
open — but it has been overtaken by #951 and is no longer evidence of anything. An open issue is not a reliable proxy for a missing feature.

## Verification

Release notes are a claim, not a result. The claim was tested against this repository: `go get github.com/mark3labs/mcp-go@v1.0.0` on `main` (755a282), then
`go build`, `go vet`, the `-short` suite, and a spike exercising the real server over both protocol eras.

### The bump costs three files

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

Consequently, on a modern connection from an elicitation-capable client, **every modifying operation fails**. The error is not
`server.ErrElicitationNotSupported`, so it does not even take the deliberate fail-closed branch (DL-6005) — it falls through to the generic
`elicitation request failed:` wrapper. Porting the gate to MRTR is therefore a hard prerequisite to advertising modern support, exactly as the earlier revision
of this ADR concluded. What changed is that it is now the _only_ substantial work, rather than one step in a whole-SDK migration.

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

The reasoning is entirely about cost, because both SDKs now implement the same revision and both bridge the two eras for us. One route is an 8-line dependency
bump that leaves every handler, tool and resource untouched and is verified green; the other rewrites 685 references across 36 files and cannot be judged
complete until the container suite passes. When two options reach the same protocol, the cheap one wins, and no argument from the earlier revision survives the
release of v1.0.0 — its case rested on `mcp-go` not having the revision.

Sequence the work in two independent changes:

1. **The bump** (v0.43.1 → v1.0.0, the three files above). Small, reviewable, and carries no protocol behaviour change for existing clients, which the SDK's own
   conformance archive pins. Landable on its own.
2. **The MRTR port of the confirmation gate**, before modern support is advertised anywhere. Until step 2 lands, a modern elicitation-capable client cannot
   perform modifying operations — see [Consequences](#consequences).

## Consequences

### The window between step 1 and step 2 is a real regression, and must be closed or fenced

After the bump, a modern client that advertises elicitation gets an error on every write. Nothing in tier-1 client shipping today reaches us that way, which is
why step 1 is safe to land first — but the gap is not hypothetical, and step 2 should follow closely. If it cannot, pin the transport to legacy-only with
`server.WithStreamableHTTPProtocolVersions(mcp.LegacyProtocolVersions()...)`, which makes the server advertise legacy versions and lets modern clients negotiate
down cleanly, instead of failing at the first write.

### We are choosing a community SDK over the reference implementation, knowingly

This is the one durable argument for `go-sdk`, and it is not answered by v1.0.0 — it is only outweighed. `mcp-go` reached this revision five weeks after
publication, and its own release notes record deliberate gaps (authorization hardening untouched; tasks not moved behind the `io.modelcontextprotocol/tasks`
extension; conformance vectors hand-written from the spec rather than pulled from the cross-SDK corpus). The deadlock documented above is a second data point
about depth of coverage. Treat sustained lag on a future revision, not this one, as the trigger to reopen the question — and reopen it as a new ADR, since the
685-reference measurement below will have gone stale by then.

### What the rejected migration would have cost

Recorded so the number is not re-derived if the question reopens. 685 SDK symbol references across 23 non-test and 13 test files:

| Package              | Refs | Files | Character of the work                                                                     |
| -------------------- | ---- | ----- | ----------------------------------------------------------------------------------------- |
| `internal/resources` | 329  | 4     | Mechanical and highly repetitive — 49 near-identical `NewResource` registrations          |
| `internal/tools`     | 132  | 18    | Tool definitions and registration; handler business logic is mostly portable              |
| `bootstrap`          | 71   | 7     | Real design work: server construction, transports, hooks → middleware, auth context funcs |
| `internal/testutils` | 65   | 2     | Test client and in-process transport                                                      |
| `internal/logging`   | 55   | 2     | Hooks become receiving middleware                                                         |
| `internal/auth`      | 16   | 1     | Middleware signature change                                                               |
| `internal/utils`     | 6    | 1     | Minimal                                                                                   |

### It must still be validated against a live TheHive

The `-short` suite passing is necessary, not sufficient. The integration suite gates on `testutils.StartTheHiveContainer`; neither step is done until it passes.
This is much less of a risk for a bump than it would have been for a rewrite, but the bump does change result marshalling on the legacy path, so it is not
skippable.

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
`NewInputRequestBuilder` and `server.ElicitationResponse`. Paying for a 685-reference rewrite to obtain a protocol we can have for eight lines needs a
justification that the maintenance argument alone does not carry.

**Bump to v1.0.0 but keep serving legacy only.** Viable as a holding position between steps 1 and 2, and named above as the fence. Rejected as a destination: it
takes the upgrade's cost without its benefit.

**Stay on v0.43.1.** Rejected. It was defensible while the alternative was a rewrite; it is not defensible against an 8-line bump. It also leaves us two
revisions behind (2025-11-25 and 2026-07-28), and forgoes the 20 intervening releases of unrelated fixes.

**Run both SDKs side by side.** Rejected: two servers to keep consistent, and the permission and confirmation layers would need duplicating — precisely the code
where divergence is most dangerous.

## What is deliberately not being done now

Three changes were prototyped alongside this investigation and **rejected as premature**, recorded here so they are not proposed again as separate work:

- **Accepting `405` in the container healthcheck.** Correct for a `2026-07-28` server, but this server cannot emit `405` until it serves the modern path, so the
  change has no effect until then and belongs with the work that enables it.
- **Consolidating the logging correlation keys.** The `session_id` placeholder only becomes noise once protocol sessions are gone, and it stays meaningful for
  as long as legacy clients are served — which, per the dual-era design, is indefinitely. Revisit only if the legacy path is retired.
- **Moving write confirmation out of the `http.RoundTripper`.** No longer premature: it is step 2, and the verification above shows why it is required rather
  than merely tidy. It should still not be done as a speculative refactor ahead of the bump, because MRTR semantics determine its shape.

### Elicitation is an optional layer, by design

Stated explicitly because step 2 passes through this code. When a client does not advertise `elicitation`, modifying requests proceed unconfirmed. **That is
intentional, not a defect.** Authorisation is the permissions configuration and the caller's TheHive API key; elicitation is one optional convenience layer
above that, and clients manage approval in their own ways. `TestRoundTrip_NoCapabilityProceeds` records the reasoning.

The MRTR port must preserve it. Turning a missing client capability into a refusal would change the security posture of every deployment using a non-elicitation
client — a product decision, not part of a protocol migration. Note that this cuts both ways after the bump: the current failure mode for modern
elicitation-capable clients is an _accidental_ refusal, arising from an SDK error that is not `ErrElicitationNotSupported`, not from a deliberate policy. Step 2
replaces the accident with the intended behaviour on both eras.

## Work, gated on accepting this ADR

1. Bump `mcp-go` to v1.0.0: type-assert in `onAfterCallToolHook`, replace the two `&struct{}{}` capability literals, `go mod tidy`. Validate against a live
   TheHive.
2. Port the confirmation gate from `http.RoundTripper` to the tool handler, expressed as MRTR input requests via `NewInputRequestBuilder` /
   `server.ElicitationResponse`. Sign `requestState` — HMAC or AEAD over the authenticated principal, a short TTL, and a digest of the originating request,
   since it travels through the client and the spec treats it as attacker-controlled. Preserve today's behaviour for clients that cannot elicit.
3. Add coverage asserting both a modern and a legacy client work, over Streamable HTTP and in-process. The spike tests above are the starting point.
4. `ttlMs`/`cacheScope` via `WithCacheHints`, resource-not-found `-32002` → `-32602`. Deterministic `tools/list` ordering is already satisfied incidentally.
5. Replace the hand-written `initialize` snippets in `docs/how-to/` with `server/discover`.
6. Report the `WithLegacyServerInitiatedRequests()` deadlock upstream.
