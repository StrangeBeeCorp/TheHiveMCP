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

Why each tool advertises these values, and why annotations are not permissions, is explained in
[Why the tools carry these annotations](../../explanation/tool-annotations.md). The mechanism that restricts access is [Permissions](../permissions.md).
