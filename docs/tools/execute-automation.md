# execute-automation

Execute Cortex analyzers and responders, or retrieve their execution status.

## Overview

The `execute-automation` tool provides integration with Cortex for running automated analysis and response actions. It allows you to execute analyzers to enrich observables with threat intelligence and run responders to perform automated actions on entities.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `operation` | string | Yes | Operation type (`run-analyzer`, `run-responder`, `get-job-status`, `get-action-status`) |
| `analyzer-id` | string | Conditional | Analyzer ID (required for `run-analyzer`) |
| `responder-id` | string | Conditional | Responder ID (required for `run-responder`) |
| `cortex-id` | string | No | Cortex instance ID (auto-routed if not specified) |
| `observable-id` | string | Conditional | Observable ID (required for `run-analyzer`) |
| `entity-type` | string | Conditional | Entity type for responders (`case`, `alert`, `task`, `observable`) |
| `entity-id` | string | Conditional | Entity ID (required for `run-responder`) |
| `job-id` | string | Conditional | Job ID (required for `get-job-status`) |
| `action-id` | string | Conditional | Action ID (required for `get-action-status`) |
| `parameters` | object | No | JSON object with automation-specific configuration |

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
  "action-id": "AWabc456"
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

## Common Analyzers

### Threat intelligence
- **VirusTotal_3_0**: File and URL reputation analysis
- **Shodan_search**: IP address and domain analysis
- **Abuse_Finder**: Email abuse contact lookup
- **DomainTools**: Domain registration and history

### File analysis
- **File_Info**: Basic file metadata extraction
- **Yara**: YARA rule matching
- **PE_Info**: Portable Executable analysis

### Network analysis
- **MaxMind**: IP geolocation lookup
- **Tor_Blutmagie**: Tor exit node detection
- **URLVoid**: URL reputation checking

## Common Responders

### Case management
- **Mailer_1_0**: Send email notifications
- **TheHive_CreateCase_1_0**: Promote alerts to cases
- **Wazuh**: Integration with Wazuh SIEM

### Threat intelligence
- **MISP_2_1**: Export IOCs to MISP
- **QRadar_2_0**: Send to IBM QRadar

### Communication
- **Slack**: Send notifications to Slack
- **Mattermost**: Send notifications to Mattermost

## Execution Workflow

### 1. Discovery phase
```json
// Get available analyzers
{
  "tool": "get-resource",
  "uri": "hive://metadata/automation/analyzers"
}

// Get analyzer documentation
{
  "tool": "get-resource",
  "uri": "hive://docs/automation/analyzers/VirusTotal_3_0"
}
```

### 2. Execution phase
```json
// Run analyzer
{
  "operation": "run-analyzer",
  "analyzer-id": "VirusTotal_3_0",
  "observable-id": "~123456"
}
```

### 3. Monitoring phase
```json
// Check job status
{
  "operation": "get-job-status",
  "job-id": "AWxyz123"
}
```

## Best Practices

### Analyzer usage
1. **Observable selection**: Ensure observables are suitable for analysis
2. **Rate limiting**: Be mindful of API rate limits for external services
3. **Parameter configuration**: Use appropriate parameters for each analyzer
4. **Result monitoring**: Check job status to ensure completion

### Responder usage
1. **Entity validation**: Verify entity exists and is accessible
2. **Permission checks**: Ensure responder has required permissions
3. **Parameter validation**: Provide correct parameters for each responder
4. **Impact assessment**: Understand what actions responders will perform

### Performance optimization
1. **Batch processing**: Group similar operations when possible
2. **Cortex routing**: Let TheHive auto-route for load balancing
3. **Status polling**: Don't over-poll job status
4. **Parameter reuse**: Cache common parameter sets

## Error Handling

Common errors and solutions:

### Analyzer errors
- **Analyzer not found**: Check available analyzers list
- **Observable not found**: Verify observable ID exists
- **Permission denied**: Ensure user can access observable
- **Rate limit exceeded**: Wait and retry with delays

### Responder errors
- **Responder not available**: Check responder compatibility with entity type
- **Entity not found**: Verify entity ID exists and is accessible
- **Configuration error**: Check responder parameters and configuration
- **External service error**: Check responder logs and external service status

### Status check errors
- **Job not found**: Verify job ID is correct and accessible
- **Timeout**: Job may still be processing - wait and retry

## Integration Patterns

### Automated analysis pipeline
1. Search for new observables
2. Run appropriate analyzers based on observable type
3. Monitor job status until completion
4. Update observables with analysis results
5. Trigger responders based on analysis outcomes

### Incident response workflow
1. Create case from alert
2. Extract observables from case evidence
3. Run threat intelligence analyzers
4. Based on results, run appropriate responders
5. Update case with analysis and response actions

### Bulk analysis
1. Search for unanalyzed observables
2. Group by type and suitable analyzers
3. Execute analyzers in batches
4. Monitor completion and handle results
5. Report on analysis coverage and findings
