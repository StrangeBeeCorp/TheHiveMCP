# TheHive Filter DSL — cheatsheet

This is everything needed to build a filter for the `search-entities` tool. The filter you provide is sent **directly** to TheHive's Query API — there is no natural-language translation step. Build it precisely.

## How a filter is shaped

A filter is a JSON object with **exactly one operator at its root**:

```json
{"_eq": {"_field": "status", "_value": "New"}}
```

Combine conditions by nesting `_and` / `_or` / `_not`, each of which takes other filters. Omit the `filters` parameter entirely (or pass `{"_any": {}}`) to match every entity within the limit.

## Operators

All operators below are verified end-to-end against TheHive.

| Operator | Shape | Meaning |
|----------|-------|---------|
| `_eq` | `{"_eq": {"_field": F, "_value": V}}` | `F` equals `V` |
| `_ne` | `{"_ne": {"_field": F, "_value": V}}` | `F` is not `V` |
| `_gt` / `_gte` | `{"_gt": {"_field": F, "_value": V}}` | `F` greater than / greater-or-equal `V` |
| `_lt` / `_lte` | `{"_lt": {"_field": F, "_value": V}}` | `F` less than / less-or-equal `V` |
| `_between` | `{"_between": {"_field": F, "_from": A, "_to": B}}` | `A ≤ F < B` — **upper bound exclusive** (half-open) |
| `_in` | `{"_in": {"_field": F, "_values": [V1, V2]}}` | `F` is one of the values (works on multi-valued fields like `tags`) |
| `_like` | `{"_like": {"_field": F, "_value": "*term*"}}` | wildcard match, case-insensitive — wildcards are `*` (e.g. `term*`, `*term`, `*term*`) |
| `_startsWith` / `_endsWith` | `{"_startsWith": {"_field": F, "_value": V}}` | `F` starts / ends with `V` |
| `_match` | `{"_match": {"_field": F, "_value": V}}` | full-text match: `V` matches a token of the analyzed text field `F` |
| `_contains` | `{"_contains": "fieldName"}` | the entity **has** that field set (presence test) — note: takes a bare field-name string, not `_field`/`_value` |
| `_id` | `{"_id": "~354"}` | match a single entity by its internal id |
| `_any` | `{"_any": {}}` | match everything |
| `_and` | `{"_and": [filter, filter, ...]}` | all sub-filters must hold |
| `_or` | `{"_or": [filter, filter, ...]}` | any sub-filter may hold |
| `_not` | `{"_not": filter}` | negation |

### `_like` vs `_match` vs `_contains`

- `_like` — substring/wildcard match on the raw field value, with `*` wildcards. Use for "title contains malware": `{"_like": {"_field": "title", "_value": "*malware*"}}`.
- `_match` — full-text search: matches whole tokens of an analyzed text field. `{"_match": {"_field": "title", "_value": "Campaign"}}` matches "Phishing Campaign".
- `_contains` — does **not** match a value; it tests that a field is present on the entity.

## Discovering valid fields

- Field names and types are **entity-specific**. Read `hive://schema/<entity-type>` (e.g. `hive://schema/alert`, `hive://schema/case`) for the exact fields before filtering.
- **Never filter on a field that does not exist.** TheHive rejects the whole query. The tool surfaces this as an error whose hint lists the valid attributes — read it and retry with a corrected filter.

## Field types and values

- **Severity** is numeric: `1=Low, 2=Medium, 3=High, 4=Critical` by default (configurable per org). Filter with numbers, e.g. `{"_gte": {"_field": "severity", "_value": 3}}`. The human-readable label lives in `severityLabel`.
- **Status / stage** are strings, e.g. alerts use `"New"`, `"Imported"`; cases use `"New"`, `"InProgress"`, `"Closed"`. Check the entity schema for valid values.
- **Tags** is a multi-valued field — use `_in` to match any of several tags, or `_eq` for an exact single tag.
- **Cases** have a human-readable `number` field (e.g. `42`) distinct from the internal `_id` (`~<n>`). "case #42" → filter `{"_eq": {"_field": "number", "_value": 42}}`, not `_id`.
- You **cannot** filter on `extraData` or on `additional-queries` results — those are enrichments applied after the search.

## Dates and relative ranges

- Date fields (`_createdAt`, `_updatedAt`, `date`, `startDate`, …) accept **ISO 8601 strings** like `"2024-08-01T00:00:00"`; the tool converts them to TheHive timestamps automatically. Epoch milliseconds also work.
- TheHive has no notion of "now" or "last week" — for relative ranges, read the current time from `hive://config/server-time`, compute the bound yourself, and pass an absolute value.
- A bounded window is a half-open `_between` (`_from` inclusive, `_to` exclusive), or combine `_gte` and `_lt`:

```json
{"_and": [
  {"_gte": {"_field": "_createdAt", "_value": "2024-08-01T00:00:00"}},
  {"_lt":  {"_field": "_createdAt", "_value": "2024-09-01T00:00:00"}}
]}
```

## Worked examples

```jsonc
// High (>=3) severity, created on/after a date
{"_and": [
  {"_gte": {"_field": "severity",   "_value": 3}},
  {"_gte": {"_field": "_createdAt", "_value": "2024-08-01T00:00:00"}}
]}

// Critical alerts still in "New" status
{"_and": [
  {"_eq": {"_field": "severity", "_value": 4}},
  {"_eq": {"_field": "status",   "_value": "New"}}
]}

// Title contains "malware" OR "phishing" (wildcard match)
{"_or": [
  {"_like": {"_field": "title", "_value": "*malware*"}},
  {"_like": {"_field": "title", "_value": "*phishing*"}}
]}

// Tagged phishing or malware
{"_in": {"_field": "tags", "_values": ["phishing", "malware"]}}

// Medium-or-higher severity that is ALSO tagged phishing or network
{"_and": [
  {"_gte": {"_field": "severity", "_value": 2}},
  {"_or": [
    {"_in": {"_field": "tags", "_values": ["phishing"]}},
    {"_in": {"_field": "tags", "_values": ["network"]}}
  ]}
]}

// Everything except low-severity noise
{"_not": {"_eq": {"_field": "severity", "_value": 1}}}

// One entity by internal id
{"_id": "~354"}
```

## Other search parameters (not part of the filter)

These are separate `search-entities` parameters, not filter operators:

- `sort-by` / `sort-order` — order results (default `_createdAt` / `desc`).
- `limit` — max rows (default 10). Not used when `count=true`.
- `extra-columns` — which columns to return; entity-specific defaults apply when omitted.
- `extra-data` — computed blocks to attach (e.g. `["taskStats"]`).
- `additional-queries` — related entities to fetch (e.g. `["tasks", "observables"]` for cases).
- `count=true` — return only the number of matches.

The applied filter is echoed back as `rawFilters` in the response. If results are unexpected, inspect `rawFilters`, re-check the field names against `hive://schema/<entity-type>`, and call again with a corrected filter.
