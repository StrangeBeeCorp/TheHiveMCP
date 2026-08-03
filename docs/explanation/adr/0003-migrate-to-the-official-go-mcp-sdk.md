# ADR-0003 - Migrate from mark3labs/mcp-go to the official modelcontextprotocol/go-sdk

- Status: proposed
- Date: August 4, 2026
- Deciders: TheHiveMCP maintainers
- Related: DL-6920, [MCP 2026-07-28 changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog)

## Context and problem statement

[MCP 2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28/changelog) is a breaking revision. It removes the `initialize` handshake, protocol
sessions and the `Mcp-Session-Id` header, the standalone HTTP `GET` stream, `ping`, `logging/setLevel` and server-initiated requests; it adds a mandatory
`server/discover`, required `Mcp-Method`/`Mcp-Name` headers, a `resultType` on every result, cache hints on list results, and Multi Round-Trip Requests (MRTR)
in place of mid-call server-to-client requests.

TheHiveMCP cannot implement any of that today, because the SDK it uses does not. This is a dependency decision before it is an engineering one.

**Nothing is broken and nothing is urgent.** A handshake-based server keeps working against dual-era clients, which is what the tier-1 clients ship. The only
decision that matters is which SDK we track, and everything else follows from it — see
[What is deliberately not being done now](#what-is-deliberately-not-being-done-now).

### Where the two Go SDKs stand

|                     | `mark3labs/mcp-go` (current)                                                                                                                                                               | `modelcontextprotocol/go-sdk`                                    |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------- |
| Version in use here | v0.43.1 → protocol **2025-06-18**                                                                                                                                                          | —                                                                |
| Latest release      | v0.57.0 → protocol 2025-11-25                                                                                                                                                              | **v1.7.0 → protocol 2026-07-28**                                 |
| 2026-07-28 support  | Not implemented. [Issue #928](https://github.com/mark3labs/mcp-go/issues/928) is open with no assignee and no target date, and explicitly scopes out MRTR, cache hints and session removal | Shipped on the revision's release day                            |
| Maintenance         | Community                                                                                                                                                                                  | Reference implementation, maintained alongside the specification |

Two facts compound. We are pinned two spec revisions behind even `mcp-go`'s own latest, and `mcp-go`'s path to the current revision has no timeline. Waiting is
not a plan with a date attached to it.

## Verification

A spike built the MCP surface TheHiveMCP needs against `go-sdk` v1.7.0 — a typed tool, a resource, receiving middleware standing in for the auth and permission
layers, and a confirmation prompt expressed as an MRTR input request. Connected over the in-memory transport, with the client supplying an elicitation handler:

```text
=== RUN   TestConfirmationRoundTrip
    mrtr_test.go:44: negotiated protocol version: "2026-07-28"
    mrtr_test.go:81: result: performed after approval, state=signed-blob-placeholder
--- PASS: TestConfirmationRoundTrip (0.00s)
```

Four things this establishes:

1. The negotiated protocol version is `2026-07-28`.
2. A tool handler returning `InputRequests` plus `RequestState` produces the round trip: the client is asked, the handler is re-invoked with `InputResponses`,
   and `RequestState` comes back verbatim.
3. The user is prompted **once**, not once per outgoing HTTP call.
4. `server/discover`, `resultType`, `CacheableResult` and the stateless Streamable HTTP path are all present in the SDK (`mcp/shared.go`, `mcp/mrtr.go`,
   `mcp/cache.go`, `mcp/streamable.go`).

### The finding that changes the migration cost

`go-sdk` ships `serverMultiRoundTripMiddleware` (`mcp/mrtr.go`). When a handler returns input requests and the connected client speaks a pre-2026-07-28
revision, the SDK fulfils them by calling the client directly and re-invokes the handler with the answers.

So **one handler implementation serves both protocol eras.** We write the confirmation gate against MRTR semantics only, and legacy clients keep working without
a second code path. That removes the dual-era branching that was the main risk in the migration, and it is the strongest argument for this SDK over waiting: it
is not merely current, it makes backwards compatibility someone else's problem.

## Decision

**Migrate to `github.com/modelcontextprotocol/go-sdk`.**

Do not spend effort on the interim `mcp-go` v0.43.1 → v0.57.0 upgrade. It would reach 2025-11-25, which is still a legacy-era revision, and every line of that
work is discarded by this migration. The only reason to do it would be if this ADR were rejected.

## Consequences

### Migration surface, measured

685 SDK symbol references across 23 non-test and 13 test files:

| Package              | Refs | Files | Character of the work                                                                     |
| -------------------- | ---- | ----- | ----------------------------------------------------------------------------------------- |
| `internal/resources` | 329  | 4     | Mechanical and highly repetitive — 49 near-identical `NewResource` registrations          |
| `internal/tools`     | 132  | 18    | Tool definitions and registration; handler business logic is mostly portable              |
| `bootstrap`          | 71   | 7     | Real design work: server construction, transports, hooks → middleware, auth context funcs |
| `internal/testutils` | 65   | 2     | Test client and in-process transport                                                      |
| `internal/logging`   | 55   | 2     | Hooks become receiving middleware                                                         |
| `internal/auth`      | 16   | 1     | Middleware signature change                                                               |
| `internal/utils`     | 6    | 1     | Minimal                                                                                   |

The count overstates the difficulty. Roughly half the references are one repetitive resource-registration pattern, and the tool handlers — where the actual
TheHive logic lives — barely touch the SDK. The genuine design work is concentrated in `bootstrap` and the middleware signatures.

### It must be validated against a live TheHive

The integration suite gates on `testutils.StartTheHiveContainer` and is skipped under `-short`. A migration of this size cannot be judged by unit tests and a
successful compile; it is not done until the container suite passes. This is the main reason to sequence it as its own change rather than folding it into
feature work.

### `hive://config/*` resource output may shift

Resource content is ours, but envelope shapes (`resultType`, cache hints, `_meta`) come from the SDK. Any client parsing the envelope rather than the content
could be affected. Worth a diff of a captured `resources/read` response before and after.

### Deliberately deferred

`ttlMs` and `cacheScope` values are a judgement call about TheHive data freshness, not a mechanical port — the static catalog tolerates a long TTL, live entity
resources do not. Land the migration with conservative values and tune separately.

## Alternatives considered

**Wait for `mcp-go` #928.** Rejected: no owner, no date, and its stated scope excludes MRTR, cache hints and session removal — the parts that actually block us.
Waiting also leaves us on 2025-06-18 while modern-only clients appear.

**Stay legacy indefinitely.** Viable for longer than it appears, because dual-era clients keep working against a handshake-based server, and this is a
self-hosted product whose clients include customer-written agents pinned to old SDKs. But it is a decision to stop tracking the protocol, and the deprecation
window on the legacy era is finite. Rejected as a destination; accepted as the current state until the migration lands.

**Run both SDKs side by side during migration.** Rejected: two servers to keep consistent, and the permission and confirmation layers would need duplicating —
precisely the code where divergence is most dangerous.

## What is deliberately not being done now

Nothing about the current server needs changing before this decision is taken. Dual-era clients work against a handshake-based server, so no deadline is being
missed, and no partial preparation is worth landing: every protocol-facing change is either impossible on the current SDK or gets rewritten by the migration
anyway.

Three changes were prototyped alongside this investigation and **rejected as premature**, recorded here so they are not proposed again as separate work:

- **Accepting `405` in the container healthcheck.** Correct for a `2026-07-28` server, but this server cannot emit `405` until it migrates, so the change has no
  effect until then and belongs inside the migration.
- **Consolidating the logging correlation keys.** The `session_id` placeholder only becomes noise once protocol sessions are gone. Do it when they are.
- **Moving write confirmation out of the `http.RoundTripper`.** Genuinely required by MRTR, and step 3 below — but only at migration time. Doing it early buys
  nothing, and invites redesigning the permission model on the way past.

### Elicitation is an optional layer, by design

Stated explicitly because the migration passes through this code. When a client does not advertise `elicitation`, modifying requests proceed unconfirmed. **That
is intentional, not a defect.** Authorisation is the permissions configuration and the caller's TheHive API key; elicitation is one optional convenience layer
above that, and clients manage approval in their own ways. `TestRoundTrip_NoCapabilityProceeds` records the reasoning.

The MRTR port must preserve it. Turning a missing client capability into a refusal would change the security posture of every deployment using a non-elicitation
client — a product decision, not part of a protocol migration.

## Work, gated on accepting this ADR

1. Port `bootstrap` (server, transports, auth context, hooks → receiving middleware).
2. Port tool and resource registration.
3. Move confirmation from the `http.RoundTripper` to the tool handler and express it as MRTR input requests, per the spike. Sign `requestState` — HMAC or AEAD
   over the authenticated principal, a short TTL, and a digest of the originating request, since it travels through the client and the spec treats it as
   attacker-controlled. Preserve today's behaviour for clients that cannot elicit.
4. Port `testutils`; add coverage asserting both a modern and a legacy client work.
5. `ttlMs`/`cacheScope`, resource-not-found `-32002` → `-32602`. Deterministic `tools/list` ordering is already satisfied incidentally.
6. Replace the hand-written `initialize` snippets in `docs/how-to/` with `server/discover`.
