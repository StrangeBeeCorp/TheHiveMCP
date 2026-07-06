# execute-automation

Execute Cortex analyzers and responders, or retrieve their execution status.

## Overview

The `execute-automation` tool provides integration with Cortex for running automated analysis and response actions. It allows you to execute analyzers to enrich
observables with threat intelligence and run responders to perform automated actions on entities.

## Parameters

| Parameter       | Type   | Required    | Description                                                                                                |
| --------------- | ------ | ----------- | ---------------------------------------------------------------------------------------------------------- |
| `operation`     | string | Yes         | Operation type (`run-analyzer`, `run-responder`, `get-job-status`, `get-action-status`)                    |
| `analyzer-id`   | string | Conditional | Analyzer ID (required for `run-analyzer`)                                                                  |
| `responder-id`  | string | Conditional | Responder ID (required for `run-responder`)                                                                |
| `cortex-id`     | string | No          | Cortex instance ID (auto-routed if not specified)                                                          |
| `observable-id` | string | Conditional | Observable ID (required for `run-analyzer`)                                                                |
| `entity-type`   | string | Conditional | Entity type (`case`, `alert`, `task`, `observable`) - required for `run-responder` and `get-action-status` |
| `entity-id`     | string | Conditional | Entity ID - required for `run-responder` and `get-action-status`                                           |
| `job-id`        | string | Conditional | Job ID (required for `get-job-status`)                                                                     |
| `action-id`     | string | Conditional | Action ID (required for `get-action-status`)                                                               |
| `parameters`    | object | No          | JSON object with automation-specific configuration                                                         |

## Operations

### Running analyzers

Analyzers enrich observables by querying external services (threat intel, reputation, etc.).

#### Basic analyzer execution

```json
{
  "operation": "run-analyzer",
  "analyzer-id": "VirusTotal_3_0",
  "observable-id": "~123456"
}
```

#### With specific Cortex instance

```json
{
  "operation": "run-analyzer",
  "analyzer-id": "VirusTotal_3_0",
  "observable-id": "~123456",
  "cortex-id": "cortex-prod-01"
}
```

#### With custom parameters

```json
{
  "operation": "run-analyzer",
  "analyzer-id": "VirusTotal_3_0",
  "observable-id": "~123456",
  "parameters": {
    "auto_extract_artifacts": true,
    "delay": 0
  }
}
```

### Running responders

Responders perform active responses on entities (block IP, send email, create ticket, etc.).

#### Case responder

```json
{
  "operation": "run-responder",
  "responder-id": "Mailer_1_0",
  "entity-type": "case",
  "entity-id": "~789"
}
```

#### Alert responder

```json
{
  "operation": "run-responder",
  "responder-id": "TheHive_CreateCase_1_0",
  "entity-type": "alert",
  "entity-id": "~456"
}
```

#### Observable responder

```json
{
  "operation": "run-responder",
  "responder-id": "MISP_2_1",
  "entity-type": "observable",
  "entity-id": "~123",
  "parameters": {
    "event_info": "Suspicious IOC from investigation",
    "analysis": "2"
  }
}
```

### Checking status

#### Get analyzer job status

```json
{
  "operation": "get-job-status",
  "job-id": "AWxyz123"
}
```

#### Get responder action status

```json
{
  "operation": "get-action-status",
  "action-id": "AWabc456",
  "entity-type": "case",
  "entity-id": "~123456"
}
```

## Automation Discovery

Before using automation, discover available analyzers and responders:

### List available analyzers

```json
{
  "tool": "get-resource",
  "uri": "hive://metadata/automation/analyzers"
}
```

### List available responders

```json
{
  "tool": "get-resource",
  "uri": "hive://metadata/automation/responders?entityType=case&entityId=~123"
}
```

### Get automation documentation

```json
{
  "tool": "get-resource",
  "uri": "hive://docs/automation/analyzers"
}
```

```json
{
  "tool": "get-resource",
  "uri": "hive://docs/automation/responders"
}
```

> **Available analyzers and responders are Cortex-catalog-specific and are not listed here.** The analyzer and responder IDs in the examples above
> (`VirusTotal_3_0`, `Mailer_1_0`, …) are illustrative — the set installed on any given deployment depends on its Cortex configuration. Query the live catalog
> with `get-resource` (see [Automation discovery](#automation-discovery)) to get the exact IDs, entity-type compatibility, and parameters your deployment
> supports.

## Notes

- **Scope.** The permission filter applies to the entity the automation acts on — the observable for `run-analyzer`, and the target entity for `run-responder` /
  `get-action-status`. For `get-job-status`, the job's target observable is resolved and scope-checked before the report is returned. Which analyzers and
  responders may run at all is gated separately by the automation allow/block lists. See the [permissions reference](../permissions.md).
- **Cortex routing.** `cortex-id` is optional; omit it to let TheHive route to the configured Cortex instance (`CORTEX_ID`, default `local`).
- **Asynchronous execution.** `run-analyzer` and `run-responder` start a job/action and return its ID. Poll `get-job-status` / `get-action-status` for
  completion.

Discover the available automation with [`get-resource`](get-resource.md), find target entities with [`search-entities`](search-entities.md), and record results
with [`manage-entities`](manage-entities.md).
