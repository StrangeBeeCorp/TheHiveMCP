# Tool annotations

Every tool this server exposes declares the four MCP annotation hints. Clients read them to decide how much ceremony a call deserves — whether to prompt the
user first, whether the tool may be auto-approved, whether a retry is safe.

## What each tool advertises

| Tool                 | `readOnlyHint` | `destructiveHint` | `idempotentHint` | `openWorldHint` |
| -------------------- | -------------- | ----------------- | ---------------- | --------------- |
| `search-entities`    | `true`         | `false`           | `true`           | `false`         |
| `get-resource`       | `true`         | `false`           | `true`           | `false`         |
| `manage-entities`    | `false`        | `true`            | `false`          | `false`         |
| `execute-automation` | `false`        | `true`            | `false`          | `true`          |

## Why these values

**The two read-only tools.** `search-entities` and `get-resource` cannot change anything: one queries TheHive, the other serves schemas and documentation from
the server's own registry. `destructiveHint` and `idempotentHint` carry no meaning while `readOnlyHint` is `true`, but the MCP payload always includes all four
keys, so they are set to the values that are trivially true of a read rather than left at a pessimistic default.

**`manage-entities` is destructive.** The hint describes what the tool _can_ do, not what a particular call does. Its operations include `delete` and `merge`,
which are irreversible, so the tool is flagged destructive even though `comment` is not.

**Only `execute-automation` is open-world.** The other three reach exactly one known, configured system — that is a closed domain however remote it is. Cortex
analyzers and responders reach an open-ended set of third-party services that the server cannot enumerate and cannot undo: submitting an observable to a
sandbox, or having a responder block an address.

## Annotations are not permissions

Annotations describe the tool; they say nothing about what a given deployment allows.

All four tools are registered unconditionally, and permissions are enforced per call. A read-only deployment still advertises `manage-entities` as mutating —
calls to it are refused at execution time, not hidden from the tool list. This matches the MCP specification, which treats annotations as untrusted hints from
the server and never as an authorization boundary. See [Permissions](../permissions.md) for the mechanism that actually restricts access.
