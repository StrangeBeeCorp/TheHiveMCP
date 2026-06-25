# TheHive Filter DSL — cheatsheet

The `search-entities` tool applies the filter you provide **directly** to TheHive.
There is no natural-language translation step — you build the filter yourself.

A filter is a JSON object with exactly **one operator at its root**. Nest
`_and` / `_or` / `_not` to combine conditions.

## Operators

| Operator | Shape | Meaning |
|----------|-------|---------|
| `_eq` | `{"_eq": {"_field": F, "_value": V}}` | field equals value |
| `_ne` | `{"_ne": {"_field": F, "_value": V}}` | field not equal to value |
| `_gt` / `_gte` | `{"_gt": {"_field": F, "_value": V}}` | greater than / or equal |
| `_lt` / `_lte` | `{"_lt": {"_field": F, "_value": V}}` | less than / or equal |
| `_between` | `{"_between": {"_field": F, "_from": A, "_to": B}}` | A ≤ field ≤ B |
| `_in` | `{"_in": {"_field": F, "_values": [V1, V2]}}` | field is one of values |
| `_like` | `{"_like": {"_field": F, "_value": "%term%"}}` | substring match (`%` wildcards) |
| `_startsWith` / `_endsWith` | `{"_startsWith": {"_field": F, "_value": V}}` | prefix / suffix match |
| `_match` | `{"_match": {"_field": F, "_value": "regex"}}` | regex match |
| `_id` | `{"_id": "~354"}` | match by internal id |
| `_any` | `{"_any": {}}` | match everything |
| `_and` | `{"_and": [filter, filter, ...]}` | all must hold |
| `_or` | `{"_or": [filter, filter, ...]}` | any may hold |
| `_not` | `{"_not": filter}` | negation |

## Fields and values

- Field names and types are **entity-specific**. Read `hive://schema/<entity-type>`
  (e.g. `hive://schema/alert`) for the exact fields before filtering. Never filter
  on a field that does not exist.
- **Severity** is numeric. TheHive's default scale is `1=Low, 2=Medium, 3=High,
  4=Critical` (configurable per org; `severityLabel` holds the label).
- **Cases** also have a human-readable `number` field (e.g. 42) distinct from the
  internal `_id` (`~<number>`). "case #42" → filter on `number`, not `_id`.
- **Dates**: use ISO strings like `"2024-08-01T00:00:00"`. For relative ranges
  ("last week"), read `hive://config/server-time` and compute the bound yourself.

## Worked examples

```jsonc
// High severity alerts created since a date
{"_and": [
  {"_gt":  {"_field": "severity",   "_value": 3}},
  {"_gte": {"_field": "_createdAt", "_value": "2024-08-01T00:00:00"}}
]}

// Critical alerts in "New" status
{"_and": [
  {"_eq": {"_field": "severity", "_value": 4}},
  {"_eq": {"_field": "status",   "_value": "New"}}
]}

// Observables with "malware" or "phishing" in the title
{"_or": [
  {"_like": {"_field": "title", "_value": "%malware%"}},
  {"_like": {"_field": "title", "_value": "%phishing%"}}
]}
```

Enrich results with related data via `additional-queries` (e.g.
`["tasks", "observables"]` for cases) and computed blocks via `extra-data`
(e.g. `["taskStats"]`, `["links"]`). The applied filter is echoed back as
`rawFilters` in the response.
