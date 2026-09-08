# search-entities

Search for entities in TheHive by providing a structured filter built from TheHive's query DSL.

## Overview

Read-only: this tool never modifies TheHive. See [Tool annotations](annotations.md).

The `search-entities` tool searches TheHive entities (alerts, cases, tasks, observables, procedures, patterns, case templates, pages, and the Cortex analyzer
and responder run history). You build the filter yourself and pass it in the `filters` parameter. There is **no** natural-language translation step and no
internal LLM: the filter you provide is applied directly to TheHive, so searches are precise and deterministic.

To build a filter you need to know the available fields and the operator grammar:

- **Fields and types** are entity-specific—read `hive://schema/<entity-type>` (for example, `hive://schema/alert`) before filtering.
- **Operator grammar**—see the [Filter DSL](#filter-dsl) below, the full JSON schema at `hive://schema/filter`, the filtering rules at `hive://rule/filtering`,
  and the worked-example cheatsheet at `hive://docs/overview/filter-dsl`.

## Parameters

| Parameter            | Type    | Required | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| -------------------- | ------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `entity-type`        | string  | Yes      | Type of entity to search for (`alert`, `case`, `task`, `observable`, `procedure`, `pattern`, `case-template`, `page`, `job`, `action`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `filters`            | object  | No       | TheHive filter object built from the query DSL (a single root operator). Omit it (or use `{"_any": {}}`) to match all entities within the limit.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `sort-by`            | string  | No       | Column to sort results by (default: `_createdAt`; `startDate` for `job` and `action`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `sort-order`         | string  | No       | Sort order `asc` or `desc` (default: `desc`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `limit`              | number  | No       | Number of results to return per page (default: 10, maximum: 1000)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `offset`             | number  | No       | Index of the first result to return, for paging (default: 0; `offset + limit` must stay under 10000). See [Paging](#paging-through-a-large-result-set).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `extra-columns`      | array   | No       | Columns to keep in output. Entity-specific defaults: alerts `['_id', 'title', '_createdAt', 'severity', 'status']`, cases `['_id', 'title', '_createdAt', 'status', 'severity']`, tasks `['_id', 'title', 'status', '_createdAt', 'assignee']`, observables `['_id', 'dataType', '_createdAt']`, procedures `['_id', 'patternId', 'patternName', 'description', 'occurDate']`, patterns `['_id', 'patternId', 'name', 'tactics', 'platforms']`, case-templates `['_id', 'name', 'displayName', '_createdAt']`, pages `['_id', 'title', 'category', '_createdAt']`, jobs `['_id', 'analyzerName', 'status', 'startDate', 'cortexId']`, actions `['_id', 'responderName', 'status', 'startDate', 'objectType', 'objectId']` |
| `extra-data`         | array   | No       | Additional data fields to include in output (see `Extra Data` in the API docs)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `additional-queries` | array   | No       | Additional queries to enrich results with related data (see `Queries available` in the API docs)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `count`              | boolean | No       | Return only the count of matching entities instead of the actual entities (default: `false`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |

## Filter DSL

A filter is a JSON object with exactly **one operator at its root**. Nest `_and` / `_or` / `_not` to combine conditions.

| Operator                    | Shape                                               | Meaning                                                            |
| --------------------------- | --------------------------------------------------- | ------------------------------------------------------------------ |
| `_eq`                       | `{"_eq": {"_field": F, "_value": V}}`               | field equals value                                                 |
| `_ne`                       | `{"_ne": {"_field": F, "_value": V}}`               | field not equal to value                                           |
| `_gt` / `_gte`              | `{"_gt": {"_field": F, "_value": V}}`               | greater than / or equal                                            |
| `_lt` / `_lte`              | `{"_lt": {"_field": F, "_value": V}}`               | less than / or equal                                               |
| `_between`                  | `{"_between": {"_field": F, "_from": A, "_to": B}}` | A ≤ field < B (**upper bound exclusive**)                          |
| `_in`                       | `{"_in": {"_field": F, "_values": [V1, V2]}}`       | field is one of values (works on multi-valued fields like `tags`)  |
| `_like`                     | `{"_like": {"_field": F, "_value": "*term*"}}`      | wildcard match, case-insensitive (`*` wildcards, **not** `%`)      |
| `_startsWith` / `_endsWith` | `{"_startsWith": {"_field": F, "_value": V}}`       | prefix / suffix match                                              |
| `_match`                    | `{"_match": {"_field": F, "_value": V}}`            | full-text match: value matches a token of the analyzed text field  |
| `_contains`                 | `{"_contains": "fieldName"}`                        | the entity has that field set (presence test on a bare field name) |
| `_id`                       | `{"_id": "~354"}`                                   | match by internal id                                               |
| `_any`                      | `{"_any": {}}`                                      | match everything                                                   |
| `_and`                      | `{"_and": [filter, filter, ...]}`                   | all must hold                                                      |
| `_or`                       | `{"_or": [filter, filter, ...]}`                    | any may hold                                                       |
| `_not`                      | `{"_not": filter}`                                  | negation                                                           |

### Fields, values and dates

- Field names and types are **entity-specific**. Read `hive://schema/<entity-type>` for exact fields before filtering. Never filter on a field that doesn't
  exist.
- **Severity** is numeric. TheHive's default scale is `1=Low, 2=Medium, 3=High, 4=Critical`, configurable per org. `severityLabel` holds the label.
- **Cases** also have a human-readable `number` field (for example, 42) distinct from the internal `_id` (`~<number>`). "case #42" → filter on `number`, not
  `_id`.
- **Dates**: use ISO strings like `"2024-08-01T00:00:00"`. For relative ranges ("last week"), read `hive://config/server-time` and compute the bound yourself.

## Examples

### Severity and status (cases)

```json
{
  "entity-type": "case",
  "filters": { "_and": [{ "_gte": { "_field": "severity", "_value": 3 } }, { "_eq": { "_field": "status", "_value": "New" } }] }
}
```

### Date range (alerts created in a window)

```json
{
  "entity-type": "alert",
  "filters": { "_between": { "_field": "_createdAt", "_from": "2024-08-01T00:00:00", "_to": "2024-08-31T23:59:59" } }
}
```

### Tags via set membership (alerts)

```json
{
  "entity-type": "alert",
  "filters": { "_in": { "_field": "tags", "_values": ["phishing", "malware"] } },
  "sort-by": "severity"
}
```

### Text match (observables whose message mentions malware or phishing)

```json
{
  "entity-type": "observable",
  "filters": { "_or": [{ "_like": { "_field": "message", "_value": "*malware*" } }, { "_like": { "_field": "message", "_value": "*phishing*" } }] }
}
```

> Observables have no `title` field—they use `data`, `dataType`, `message`, `tags`. Always check `hive://schema/observable` for valid fields.

### Latest N of an entity (no filter)

```json
{
  "entity-type": "task",
  "limit": 3
}
```

## Supported Entity Types

### Alerts

Search for security alerts with filters on type, source, severity, tags, creation/update dates, status, and assignee.

### Cases

Search for investigation cases with filters on title, description, severity, status, stage, assignee, tags, custom fields, and creation/resolution dates.

### Tasks

Search for case tasks with filters on title, description, status, assignee, group, due dates, and completion.

### Observables

Search for artifacts and IOCs with filters on data type, value, tags, analysis results, creation/update dates, and associated cases or alerts.

### Procedures

Search for TTP entries (MITRE ATT&CK mappings) attached to cases or alerts with filters on pattern ID, pattern name, tactic, occurrence date, and description.

### Patterns

Search the MITRE ATT&CK technique catalog loaded in TheHive with filters on:

- Pattern ID (e.g. `T1059`, `T1059.001`)
- Name and description
- Tactics (e.g. `execution`, `persistence`)
- Platforms (e.g. `Windows`, `Linux`)
- `revoked` status

**Tip**: Search patterns first to find valid `patternId` values before creating procedures.

### Case Templates

Search for reusable case blueprints with filters on name, display name, description content, tags, and creation/update dates.

### Pages

Search for documentation pages (standalone or case-attached) with filters on title, content, category, and creation/update dates.

### Jobs (analyzer runs)

Search the history of Cortex analyzer runs with filters on `analyzerName`, `analyzerId`, `analyzerDefinition`, `cortexId`, `status` (`Waiting`, `InProgress`,
`Success`, `Failure`, `Deleted`) and `startDate`. Use this to find out what enrichment already happened instead of re-running an analyzer.

**Match analyzers on `analyzerName`** (`VirusTotal_GetReport_3_1`). The two neighbouring fields look usable and are not: `analyzerId` holds an opaque Cortex id
(`bc265b6aa7d3131998bb6125d548d880`), and `analyzerDefinition` holds the same versioned value as `analyzerName` rather than a version-agnostic one — so
filtering either with a plain analyzer name returns zero rows and no error. For version-agnostic matching use a wildcard on the name:
`{"_like": {"_field": "analyzerName", "_value": "VirusTotal*"}}`.

Dates sort and filter on `startDate` (when the run happened, and the default sort for this type); `_createdAt` also works.

Analyzer reports are omitted by default — TheHive attaches a partial report to every job row, which is bulky untrusted third-party content. Add
`"extra-data": ["report"]` to get the full report. Requires TheHive's Cortex connector to be enabled.

### Actions (responder runs)

Search the history of Cortex responder runs with filters on `responderName`, `responderId`, `objectId`, `cortexId`, `status` and `startDate`. As with jobs,
`responderId` is an opaque Cortex id, dates behave the same way, and the Cortex connector must be enabled.

`objectType` is returned (as `Case`, `Alert`, …) but **cannot be filtered on**: TheHive derives it from a graph traversal rather than storing it, so any filter
on it silently matches nothing — not even the value it just returned. Scope by `objectId`, or search the target entity and expand it with
`additional-queries: ["actions"]`.

To list the runs of a single entity, prefer the `additional-queries` expansions (`jobs`, `actions`) over filtering the history by target — see below.

## Advanced Usage

### Count-only queries

Get only the total count of matching entities without returning the actual data:

```json
{
  "entity-type": "case",
  "filters": { "_gte": { "_field": "severity", "_value": 3 } },
  "count": true
}
```

Response:

```json
{
  "count": 42,
  "countOnly": true,
  "offset": 0,
  "hasMore": false,
  "entityType": "case",
  "rawFilters": {...}
}
```

A count-only query aggregates the whole match server-side, so there is no window: `offset` is always `0` and `hasMore` always `false`, whatever `limit` and
`offset` were passed.

### Paging through a large result set

Results are a window over the match, never necessarily the whole of it. `count` is the number of rows on **this page**, not the size of the result set.

Every response carries `hasMore`. When it is `true` the match continues past this page, and `nextOffset` holds the `offset` to pass to the next call — every
other parameter unchanged. When it is `false` this page reaches the end, so `offset + count` is the total, and `nextOffset` is absent.

```json
{
  "entity-type": "alert",
  "filters": { "_eq": { "_field": "status", "_value": "New" } },
  "limit": 50
}
```

Response:

```json
{
  "count": 50,
  "countOnly": false,
  "offset": 0,
  "hasMore": true,
  "nextOffset": 50,
  "entityType": "alert",
  "results": [...],
  "rawFilters": {...}
}
```

Read the next page by echoing `nextOffset` back as `offset`:

```json
{
  "entity-type": "alert",
  "filters": { "_eq": { "_field": "status", "_value": "New" } },
  "limit": 50,
  "offset": 50
}
```

Detecting truncation costs nothing extra: the server asks TheHive for one row beyond `limit` and drops it before returning the page.

Two caveats:

- **Set `sort-by` and `sort-order` explicitly when paging.** Pages are separate queries; rows created or updated between them can shift the ordering and cause a
  row to be served twice or skipped.
- **`offset + limit` must stay under 10000.** The bound is on the result _window_ — the first row plus the rows read — not on `offset` alone, so a large `limit`
  reaches it sooner: `offset: 9500, limit: 1000` is refused even though the offset is well under 10000. Beyond the window TheHive's index refuses the query
  outright, so the server rejects it up front with a message saying so. Narrow the filter — or use `count: true` if all you need is the size of the match, which
  is exact and unaffected by `limit` and `offset`.

### Custom columns

Specify which fields to return in the results:

```json
{
  "entity-type": "case",
  "filters": { "_gte": { "_field": "severity", "_value": 3 } },
  "extra-columns": ["_id", "title", "severity", "assignee", "status"]
}
```

### Additional data

Include computed extra-data blocks:

```json
{
  "entity-type": "alert",
  "filters": { "_like": { "_field": "title", "_value": "*phishing*" } },
  "extra-data": ["status", "procedureCount"]
}
```

### Related data queries

Enrich results with related information:

```json
{
  "entity-type": "case",
  "additional-queries": ["tasks", "observables", "procedures"]
}
```

Cases, alerts and tasks also support `actions` (responder runs). Observables support `jobs` (analyzer runs) and `actions`, which is the direct way to answer
"what enrichment already ran on this observable?". Expansion results are projected through the entity's default columns, so a `jobs` row carries `_id`,
`analyzerName`, `status`, `startDate` and `cortexId` — enough to see what ran and whether it succeeded, but not what it returned. For report content, search
`entity-type: "job"` directly with `extra-data: ["report"]`:

```json
{
  "entity-type": "observable",
  "filters": { "_id": "~123456" },
  "additional-queries": ["jobs"]
}
```

## Notes

- **`rawFilters` echo.** The applied filter is echoed back in every response. When results are unexpected, inspect `rawFilters` to see exactly what was sent to
  TheHive.
- **Field discovery.** Valid fields and types per entity are defined in `hive://schema/<entity-type>` (output schema). The `/create` and `/update` variants
  describe the fields [`manage-entities`](manage-entities.md) accepts. Filtering on a field an entity type doesn't have fails.
- **Grammar reference.** The full operator grammar is at `hive://schema/filter` and the worked-example cheatsheet at `hive://docs/overview/filter-dsl`.
- **Related tools.** Discover fields and automation options with [`get-resource`](get-resource.md). Act on results with [`manage-entities`](manage-entities.md)
  and [`execute-automation`](execute-automation.md).

For a task-oriented walkthrough of running a search end to end, see the [first-investigation tutorial](../../tutorial/first-investigation.md).
