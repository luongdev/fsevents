# Regex Filter Implementation

## Overview

The FreeSWITCH ESL Sidecar App now supports regex-based event filtering using the `regex` operator. This allows for sophisticated pattern matching on event headers and fields.

## Implementation Details

### Files Modified

1. **`internal/processor/filter.go`**
   - Added `regexp` import
   - Implemented regex case in `evaluateFilter()` method
   - Added comprehensive error handling for invalid regex patterns
   - Added debug logging for regex evaluation

2. **`internal/processor/filter_test.go`**
   - Added comprehensive test cases for regex functionality
   - Tests cover valid/invalid patterns, case sensitivity, multiple filters
   - Tests edge cases like invalid regex patterns

### Features

- ✅ **Basic regex matching** with Go's `regexp` package
- ✅ **Case-insensitive matching** using `(?i)` flag
- ✅ **Error handling** for invalid regex patterns
- ✅ **Debug logging** for troubleshooting
- ✅ **Multiple filter support** with AND logic
- ✅ **Comprehensive testing** with various scenarios

### Supported Regex Features

The implementation uses Go's `regexp` package, which supports:

- **Character classes**: `[a-z]`, `[0-9]`, `\d`, `\w`, `\s`
- **Quantifiers**: `+`, `*`, `?`, `{n}`, `{n,m}`
- **Anchors**: `^` (start), `$` (end)
- **Groups**: `(pattern)`, `(?:pattern)` (non-capturing)
- **Alternation**: `pattern1|pattern2`
- **Case-insensitive**: `(?i)pattern`
- **Flags**: `(?flags)pattern`

### Limitations

Go's regex engine does **NOT** support:
- ❌ **Lookahead/lookbehind**: `(?=...)`, `(?!...)`, `(?<=...)`, `(?<!...)`
- ❌ **Backreferences**: `\1`, `\2`
- ❌ **Conditional expressions**: `(?(condition)yes|no)`

## Configuration Examples

### Basic Phone Number Validation

```yaml
events:
  filters:
    - field: "Caller-Caller-ID-Number"
      operator: "regex"
      value: "^\\+?1?[2-9]\\d{2}[2-9]\\d{2}\\d{4}$"  # US phone format
```

### UUID Format Validation

```yaml
events:
  filters:
    - field: "Unique-ID"
      operator: "regex"
      value: "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"
```

### Case-Insensitive User Agent Filtering

```yaml
events:
  filters:
    - field: "variable_sip_user_agent"
      operator: "regex"
      value: "(?i)bot|scanner|crawler|attack"  # Block bad user agents
```

### Call Direction Validation

```yaml
events:
  filters:
    - field: "Call-Direction"
      operator: "regex"
      value: "^(inbound|outbound)$"  # Only valid directions
```

### Multiple Regex Filters (AND Logic)

```yaml
events:
  filters:
    # Must be valid US phone number
    - field: "Caller-Caller-ID-Number"
      operator: "regex"
      value: "^\\+?1?[2-9]\\d{2}[2-9]\\d{2}\\d{4}$"
    
    # Must be known good device
    - field: "variable_sip_user_agent"
      operator: "regex"
      value: "(?i)polycom|cisco|yealink"
    
    # Must be valid call direction
    - field: "Call-Direction"
      operator: "regex"
      value: "^(inbound|outbound)$"
```

## Error Handling

### Invalid Regex Patterns

If a regex pattern is invalid, the filter will:
1. Log an ERROR message with details
2. Return `false` (reject the event)
3. Continue processing other filters

Example error log:
```json
{
  "level": "error",
  "logger": "filter",
  "msg": "Invalid regex pattern",
  "field": "Caller-Caller-ID-Number",
  "pattern": "[invalid-regex",
  "error": "error parsing regexp: missing closing ]: `[invalid-regex`"
}
```

### Debug Logging

When log level is set to `debug`, regex evaluation will be logged:

```json
{
  "level": "debug",
  "logger": "filter",
  "msg": "Regex filter evaluation",
  "field": "Caller-Caller-ID-Number",
  "pattern": "^\\+?1?[2-9]\\d{2}[2-9]\\d{2}\\d{4}$",
  "actual_value": "+12125551234",
  "matched": true
}
```

## Performance Considerations

1. **Regex Compilation**: Each regex pattern is compiled on every filter evaluation. For high-throughput scenarios, consider caching compiled regexes.

2. **Complex Patterns**: Avoid overly complex regex patterns that could cause performance issues.

3. **Filter Order**: Place more restrictive/faster filters first to short-circuit evaluation.

## Testing

### Running Tests

```bash
# Test regex functionality
go test ./internal/processor -v -run TestEventFilter_RegexOperator

# Test multiple filters with regex
go test ./internal/processor -v -run TestEventFilter_MultipleFiltersWithRegex

# Run demo
go run examples/regex_filter_demo.go
```

### Test Coverage

The implementation includes tests for:
- ✅ Valid regex patterns matching
- ✅ Invalid patterns not matching
- ✅ Case-insensitive matching
- ✅ Invalid regex pattern handling
- ✅ Multiple filter combinations
- ✅ Edge cases and error scenarios

## Common Regex Patterns

### Phone Numbers

```yaml
# US Phone Number (with optional +1)
value: "^\\+?1?[2-9]\\d{2}[2-9]\\d{2}\\d{4}$"

# International format
value: "^\\+[1-9]\\d{1,14}$"

# Any digits only
value: "^\\d+$"
```

### UUIDs and IDs

```yaml
# Standard UUID format
value: "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"

# Alphanumeric ID
value: "^[a-zA-Z0-9]+$"
```

### IP Addresses

```yaml
# Simple IPv4 (basic validation)
value: "^\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}$"

# IPv6 (simplified)
value: "^[0-9a-fA-F:]+$"
```

### User Agents

```yaml
# Block known bad patterns (case-insensitive)
value: "(?i)bot|crawler|scanner|attack|hack|exploit"

# Allow only known good devices
value: "(?i)^(polycom|cisco|yealink|avaya)"
```

## Integration with Other Operators

Regex filters work alongside other filter operators:

```yaml
events:
  filters:
    # Exact match
    - field: "Event-Name"
      operator: "equals"
      value: "CHANNEL_CREATE"
    
    # Regex match
    - field: "Caller-Caller-ID-Number"
      operator: "regex"
      value: "^\\+?1?[2-9]\\d{2}[2-9]\\d{2}\\d{4}$"
    
    # Contains match
    - field: "Channel-State"
      operator: "contains"
      value: "ACTIVE"
```

## Best Practices

1. **Escape Special Characters**: Remember to escape regex special characters in YAML:
   ```yaml
   # Correct
   value: "^\\+?1?[2-9]\\d{2}[2-9]\\d{2}\\d{4}$"
   
   # Incorrect  
   value: "^\+?1?[2-9]\d{2}[2-9]\d{2}\d{4}$"
   ```

2. **Use Anchors**: Always use `^` and `$` for exact matching:
   ```yaml
   # Good - matches exactly "inbound" or "outbound"
   value: "^(inbound|outbound)$"
   
   # Bad - would match "some-inbound-call" 
   value: "(inbound|outbound)"
   ```

3. **Case Sensitivity**: Use `(?i)` for case-insensitive matching:
   ```yaml
   # Case-insensitive
   value: "(?i)polycom|cisco"
   ```

4. **Test Patterns**: Always test regex patterns with sample data before deploying.

5. **Document Patterns**: Add comments explaining complex regex patterns:
   ```yaml
   - field: "Caller-Caller-ID-Number"
     operator: "regex"
     value: "^\\+?1?[2-9]\\d{2}[2-9]\\d{2}\\d{4}$"  # US phone format: +1234567890
   ```

## Migration from Other Operators

You can often replace simpler operators with regex for more flexibility:

```yaml
# Before: exact match
- field: "Call-Direction"
  operator: "equals"
  value: "inbound"

# After: regex with multiple options
- field: "Call-Direction"
  operator: "regex"
  value: "^(inbound|outbound)$"
```

```yaml
# Before: starts_with
- field: "variable_sip_user_agent"
  operator: "starts_with"
  value: "Polycom"

# After: case-insensitive regex
- field: "variable_sip_user_agent"
  operator: "regex"
  value: "(?i)^polycom"
``` 