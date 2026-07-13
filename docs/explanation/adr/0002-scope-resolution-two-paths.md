# ADR-0002 - Two scope-resolution paths: tolerant get-by-ID for user input, batched `_in` for expansion

- Status: accepted
- Date: July 13, 2026
- Deciders: TheHiveMCP maintainers
- Related: [permissions reference](../../reference/permissions.md), DL-5764, DL-6004

## Context and problem statement

Permission filters restrict which entities a tool may reach (see the [permissions reference](../../reference/permissions.md)). Enforcement is a _scope check_:
before an operation touches an entity, the server asks TheHive whether that entity still matches the configured filter, and denies the operation if it does not.
The filter is always evaluated by TheHive; the server only decides which query to send.

Two different callers need this check, and they receive their entity IDs from very different places:

1. **The manage and execute-automation tools** act on IDs supplied by the user or the LLM. The tool schema advertises human-friendly identifiers — a bare case
   number, a case-template name — alongside the internal `_id`, and the pre-DL-5764 behavior resolved all of them.
2. **Similarity expansion** re-scopes the hits returned by a prior search (a case's similar cases or alerts). Those IDs come straight out of TheHive's own query
   results, so they are always the internal `_id` form.

DL-5764 set out to make scope resolution cheaper for the expansion path: a single similarity expansion can return dozens of hits, and checking each with its own
round-trip is N queries where one would do. The optimization batches all IDs into one list query with an `_in` filter over `_id`:

```text
listCase → filter(_and[ permFilters, _in{_field:_id, _values:[…ids…]} ]) → page
```

The problem: that batched query was applied to **every** caller, including the manage and execute-automation paths — and `_in{_id}` does not behave like the
get-by-ID resolution it replaced.

## The two ID forms, and why `_in` rejects one of them

TheHive entities carry two distinct identifiers:

- **`number`** — a per-organization sequential business counter shown in the UI (case #3, alert #17). Small, human-facing.
- **`_id`** — the internal database identifier, a JanusGraph vertex id rendered with a leading `~` (for example `~40988160`).

**These are unrelated values.** `~` + `number` is not the `_id`: case #3 might be `~40988160`. There is no arithmetic or string transformation from one to the
other; the mapping lives only in TheHive.

The two query shapes treat a bare identifier differently:

- **Get-by-ID** (`getCase idOrName=<id> → filter`) is _tolerant_: `idOrName` accepts the internal `_id`, a bare `number`, or a name, and TheHive resolves
  whichever it was given.
- **`_in{_field:_id}`** on a list op matches **only** the internal `_id` form, byte-for-byte. A bare `number` never matches, because it is simply not that
  entity's `_id`.

So an entity that the old get-by-ID path resolved could silently fail the `_in` match. `inScope[id]` came back `false`, and a **legitimate operation was
denied** — a case update addressed by its case number, for instance. This is fail-closed (a false denial, never a leak), so it is not a security regression, but
it is a functional one on a path where bare IDs are a documented, supported input.

## Considered options

1. **One batched `_in` path for every caller** _(the DL-5764 regression)_. Fast, single mechanism — but it silently denies any bare identifier on the
   user-facing paths. Rejected: it breaks a supported input.
2. **Normalize bare IDs to `~`-prefixed before the check.** Keep the fast `_in` path everywhere and map bare input to `_id` first. Rejected: `~` + `number` is
   _not_ the `_id` (see above), so prefixing is a guess that matches nothing or, worse, the wrong entity; and it cannot help name-based identifiers at all. It
   trades a clean denial for a fragile heuristic.
3. **Route each caller to the query whose precondition it satisfies** _(chosen)_. User-facing paths use tolerant get-by-ID; the expansion path keeps the batched
   `_in`.

## Decision

Scope resolution uses **two paths, each applied where its input guarantee holds**:

- **Manage and execute-automation** resolve scope with the tolerant get-by-ID path, one query per ID (`getX idOrName=<id> → filter(permFilters)`). These paths
  accept user/LLM-supplied IDs that may be bare, so they need the forgiving resolution. They address only a handful of IDs per call, so the per-ID round-trips
  cost little.
- **Similarity expansion** keeps the batched single-query `_in` path (`listX → filter(_and[permFilters, _in{_id}]) → page`). Its IDs come from prior search
  results and are provably the `~`-prefixed `_id` form, so the batch's precondition is always met — and this is the path where collapsing N hits into one
  round-trip actually matters.

In code, the tolerant resolver is exposed as `utils.ScopedEntityIDsTolerant` / `IsEntityInScopeTolerant`; the batched resolver as `utils.GetEntityIDsInScope` /
`GetScopedEntityIDsBatch`. Each resolver documents its ID-form precondition so the choice is visible at the call site. A live integration test drives the manage
path with a **bare** case number and asserts the in-scope operation succeeds, locking the regression out; a differential test pins the `_in` path against the
get-by-ID path on `~`-prefixed IDs so the two never diverge on their shared input.

## Consequences

**Good**: bare identifiers work again on the user-facing paths, matching the [permissions reference](../../reference/permissions.md) and the tool schema.
Expansion keeps its single-round-trip batching. Both failure modes stay covered — false denial on the manage path and leak/over-match on the expansion path each
have a dedicated live test.

**Trade-offs**: two resolution paths instead of one, and the manage/execute-automation paths issue one query per ID rather than a single batched query. Both are
deliberate: the second path exists precisely because its callers' inputs differ, and the per-ID cost is negligible at the handful-of-IDs scale those tools
operate on. The batched path is reserved for the only caller whose IDs are guaranteed to satisfy its stricter precondition.
