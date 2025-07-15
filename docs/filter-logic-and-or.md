# Filter Logic: AND vs OR

## Overview

The FreeSWITCH ESL Sidecar App supports configurable filter logic to control how multiple event filters are evaluated. You can choose between **AND logic** (all filters must match) or **OR logic** (any filter can match).

## Configuration

### Basic Configuration

```yaml
events:
  filter_logic: "AND"  # or "OR" - defaults to "AND" if not specified
  
  filters:
    - field: "Event-Name"
      operator: "equals"
      value: "CHANNEL_CREATE"
    
    - field: "Call-Direction"
      operator: "equals"
      value: "inbound"
```

### Valid Values

- `"AND"` (default) - All filters must match for an event to pass
- `"OR"` - Any filter can match for an event to pass
- Empty or omitted - Defaults to `"AND"`

## AND Logic (Default)

### How it Works

With AND logic, an event **must match ALL filters** to be processed. If any single filter fails, the event is rejected.

```yaml
events:
  filter_logic: "AND"  # Optional - this is the default
  
  filters:
    - field: "CC-Action"
      operator: "regex"
      value: "^(agent-)(status|state)(-change)$"
    
    - field: "CC-Agent-State"
      operator: "not_equals"
      value: "Unknown"
```

### Examples

| CC-Action | CC-Agent-State | Result | Reason |
|-----------|----------------|--------|--------|
| `agent-status-change` | `Available` | ✅ **Pass** | Both filters match |
| `agent-status-change` | `Unknown` | ❌ **Fail** | Second filter fails |
| `agent-login` | `Available` | ❌ **Fail** | First filter fails |
| `agent-login` | `Unknown` | ❌ **Fail** | Both filters fail |

### Use Cases

- **Quality Control**: Events must meet multiple strict criteria
- **Compliance**: All security requirements must be satisfied
- **Precision Filtering**: Need very specific event combinations

```yaml
# Example: Quality Assurance Monitoring
events:
  filter_logic: "AND"
  
  filters:
    # Must be completed calls
    - field: "Hangup-Cause"
      operator: "equals"
      value: "NORMAL_CLEARING"
    
    # Must be longer than 30 seconds
    - field: "variable_billsec"
      operator: "greater_than"
      value: "30"
    
    # Must be from QA agents
    - field: "CC-Agent"
      operator: "starts_with"
      value: "qa-agent"
```

## OR Logic

### How it Works

With OR logic, an event **passes if ANY filter matches**. The event only gets rejected if ALL filters fail.

```yaml
events:
  filter_logic: "OR"
  
  filters:
    - field: "CC-Action"
      operator: "regex"
      value: "^(agent-)(status|state)(-change)$"
    
    - field: "Event-Name"
      operator: "equals"
      value: "HEARTBEAT"
```

### Examples

| CC-Action | Event-Name | Result | Reason |
|-----------|------------|--------|--------|
| `agent-status-change` | `CUSTOM` | ✅ **Pass** | First filter matches |
| `agent-login` | `HEARTBEAT` | ✅ **Pass** | Second filter matches |
| `agent-status-change` | `HEARTBEAT` | ✅ **Pass** | Both filters match |
| `agent-login` | `CHANNEL_CREATE` | ❌ **Fail** | No filters match |

### Use Cases

- **Monitoring Multiple Event Types**: Want different types of events
- **Alerting**: Trigger on various critical conditions
- **Broad Collection**: Gather events that meet any of several criteria

```yaml
# Example: Critical Event Monitoring
events:
  filter_logic: "OR"
  
  filters:
    # Agent logout events
    - field: "CC-Action"
      operator: "equals"
      value: "agent-logout"
    
    # Failed calls
    - field: "Hangup-Cause"
      operator: "regex"
      value: "^(USER_BUSY|NO_ANSWER|CALL_REJECTED)$"
    
    # High system load
    - field: "Session-Count"
      operator: "greater_than"
      value: "1000"
```

## Complex Examples

### Multi-Queue Support with OR Logic

```yaml
# Monitor agents from multiple queues
events:
  filter_logic: "OR"
  
  filters:
    # Support queue agents
    - field: "CC-Queue"
      operator: "equals"
      value: "support"
    
    # Sales queue agents  
    - field: "CC-Queue"
      operator: "equals"
      value: "sales"
    
    # Manager override - any manager action
    - field: "CC-Agent"
      operator: "starts_with"
      value: "manager-"
```

### Comprehensive Call Tracking with AND Logic

```yaml
# Track only important completed calls
events:
  filter_logic: "AND"
  
  filters:
    # Only specific call events
    - field: "Event-Name"
      operator: "regex"
      value: "^CHANNEL_(CREATE|HANGUP)$"
    
    # Only external calls (not internal)
    - field: "Caller-Caller-ID-Number"
      operator: "regex"
      value: "^\\+?[1-9]\\d{7,14}$"  # International format
    
    # Only business hours (would need custom logic)
    - field: "Event-Date-Local"
      operator: "contains"
      value: "T0[89]:|T1[0-7]:"  # 8 AM to 5 PM
```

### Mixed Logic with Multiple Filter Groups

For complex scenarios, you might need multiple configurations:

```yaml
# config-critical-events.yaml
events:
  filter_logic: "OR"  # Any critical event
  filters:
    - field: "CC-Action"
      operator: "equals"
      value: "agent-logout"
    - field: "Hangup-Cause"
      operator: "not_equals"
      value: "NORMAL_CLEARING"

# config-quality-calls.yaml  
events:
  filter_logic: "AND"  # All criteria must match
  filters:
    - field: "Hangup-Cause"
      operator: "equals"
      value: "NORMAL_CLEARING"
    - field: "variable_billsec"
      operator: "greater_than"
      value: "30"
```

## Performance Considerations

### AND Logic Performance
- **Short-circuit evaluation**: Stops at first failed filter
- **More restrictive**: Fewer events pass through
- **Better for high-volume environments**

### OR Logic Performance  
- **May evaluate more filters**: Continues until one matches
- **More permissive**: More events pass through
- **Consider downstream processing capacity**

## Debugging Filter Logic

### Enable Debug Logging

```yaml
logging:
  level: "debug"  # Shows filter evaluation details
  format: "console"
```

### Log Output Examples

**AND Logic Debug:**
```
DEBUG filter Evaluating filters {"event_name": "CUSTOM", "filter_count": 2, "filter_logic": "AND"}
DEBUG filter Event filtered out by AND logic {"filter_field": "CC-Agent-State", "filter_operator": "not_equals"}
```

**OR Logic Debug:**
```
DEBUG filter Evaluating filters {"event_name": "CUSTOM", "filter_count": 2, "filter_logic": "OR"}  
DEBUG filter Event passed OR filter {"filter_field": "CC-Action", "filter_operator": "regex"}
```

## Migration Guide

### From Implicit AND to Explicit Configuration

**Before:**
```yaml
events:
  filters:
    - field: "Event-Name"
      operator: "equals"
      value: "HEARTBEAT"
```

**After (Explicit AND):**
```yaml
events:
  filter_logic: "AND"  # Explicitly specify logic
  filters:
    - field: "Event-Name"
      operator: "equals"
      value: "HEARTBEAT"
```

**After (Change to OR):**
```yaml
events:
  filter_logic: "OR"  # Change behavior to OR
  filters:
    - field: "Event-Name"
      operator: "equals"
      value: "HEARTBEAT"
    - field: "Event-Name"
      operator: "equals"
      value: "CHANNEL_CREATE"
```

## Error Handling

### Invalid Filter Logic
```yaml
events:
  filter_logic: "INVALID"  # ❌ Error: must be 'AND' or 'OR'
```

**Error Message:**
```
filter_logic must be 'AND' or 'OR', got: INVALID
```

### Validation Command
```bash
# Validate your configuration
go run cmd/fsevents/main.go config validate --config your-config.yaml
```

## Best Practices

1. **Default to AND**: Use AND logic for precise filtering
2. **Use OR for Alerts**: OR logic works well for monitoring multiple critical conditions
3. **Document Logic**: Add comments explaining why you chose AND vs OR
4. **Test Thoroughly**: Use debug logging to verify filter behavior
5. **Consider Performance**: OR logic may process more events
6. **Group Related Filters**: Keep logically related filters together

## Complete Configuration Example

```yaml
# FreeSWITCH ESL Configuration with OR Filter Logic
esl:
  host: "192.168.13.137"
  port: 8021
  password: "ClueCon"
  timeout: 10s

events:
  subscribe_events:
    - "CUSTOM callcenter::info"
    - "HEARTBEAT"
    - "CHANNEL_HANGUP"

  # OR Logic: Accept any of the following event types
  filter_logic: "OR"
  
  filters:
    # Critical agent events
    - field: "CC-Action"
      operator: "regex"
      value: "^(agent-)(logout|state-change)$"
    
    # System monitoring
    - field: "Event-Name"
      operator: "equals"
      value: "HEARTBEAT"
    
    # Failed calls
    - field: "Hangup-Cause"
      operator: "not_equals"
      value: "NORMAL_CLEARING"

  field_mappings:
    - from: "Event-Date-Timestamp"
      to: "timestamp"
      transforms: [":int"]

http:
  destinations:
    - name: "monitoring_webhook"
      url: "https://your-monitoring-system.com/webhooks/freeswitch"
      method: "POST"
      headers:
        Content-Type: "application/json"
        X-Filter-Logic: "OR"  # Document the logic used
      timeout: 30s

logging:
  level: "debug"  # Enable to see filter evaluation
  format: "console"

metrics:
  enabled: true
  port: 9090
``` 