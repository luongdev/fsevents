# Per-Destination Payload Templates

## Tổng quan

Tính năng Per-Destination Payload Templates cho phép bạn cấu hình định dạng payload khác nhau cho từng HTTP destination. Thay vì sử dụng một payload template global cho tất cả destinations, giờ đây mỗi destination có thể có template riêng biệt.

## Cấu hình

### Cấu trúc Payload Template

```yaml
payload_template:
  format: "json"        # json, xml, form (mặc định: json)
  template: ""          # Go template string (tùy chọn)
  headers:              # Headers bổ sung (tùy chọn)
    X-Custom-Header: "value"
```

### Các định dạng được hỗ trợ

1. **JSON** (`json`): Định dạng mặc định, tạo JSON object
2. **XML** (`xml`): Tạo XML document (đang phát triển)
3. **Form** (`form`): Tạo application/x-www-form-urlencoded (đang phát triển)

### Ví dụ cấu hình

```yaml
http:
  destinations:
    # Analytics API - JSON với headers tùy chỉnh
    - name: "analytics-api"
      url: "https://api.analytics.com/events"
      method: "POST"
      headers:
        Content-Type: "application/json"
        Authorization: "Bearer token"
      timeout: 30s
      event_filters: ["CUSTOM callcenter::*"]
      
      payload_template:
        format: "json"
        headers:
          X-Event-Source: "freeswitch-esl"
          X-Analytics-Version: "v1.0"
          X-Tenant-ID: "tenant-123"

    # Call tracking - XML format
    - name: "call-tracker"
      url: "https://tracker.example.com/calls"
      method: "POST"
      headers:
        Content-Type: "application/xml"
      timeout: 15s
      event_filters: ["CHANNEL_*"]
      
      payload_template:
        format: "xml"
        headers:
          X-Schema-Version: "1.0"

    # Form endpoint
    - name: "form-processor"
      url: "https://form.example.com/submit"
      method: "POST"
      headers:
        Content-Type: "application/x-www-form-urlencoded"
      timeout: 10s
      event_filters: ["HEARTBEAT"]
      
      payload_template:
        format: "form"
        headers:
          X-Form-Type: "event-data"

    # Default JSON (không có payload_template)
    - name: "default-endpoint"
      url: "https://webhook.example.com"
      method: "POST"
      headers:
        Content-Type: "application/json"
      timeout: 30s
      event_filters: ["*"]
      # Không có payload_template - sử dụng JSON mặc định
```

## Cách hoạt động

### 1. Xử lý payload theo destination

Mỗi khi có event được forward:

1. **Field Mapping**: Event được áp dụng field mappings global
2. **Custom Processors**: Event đi qua các custom processors
3. **Payload Template**: Payload được format theo template của destination cụ thể
4. **HTTP Request**: Gửi request với format đã được chỉ định

### 2. Headers tùy chỉnh

Headers từ `payload_template.headers` sẽ được thêm vào HTTP request:

```yaml
payload_template:
  format: "json"
  headers:
    X-Event-Source: "freeswitch"     # Thêm vào request
    X-Version: "1.0"                 # Thêm vào request
    X-Timestamp: "{{.timestamp}}"    # Template variables (tương lai)
```

### 3. Fallback behavior

- Nếu destination không có `payload_template`: Sử dụng JSON mặc định
- Nếu format không được hỗ trợ: Log warning và fallback về JSON
- Nếu template processing thất bại: Sử dụng payload gốc

## Debug và Logging

### Debug logs

Với `logging.level: "debug"`, bạn có thể thấy:

```log
DEBUG   http    HTTP request body   {
  "destination": "analytics-api",
  "url": "https://api.analytics.com/events",
  "method": "POST",
  "request_body": "{\"timestamp\":\"2024-01-15T10:30:00Z\",\"action\":\"agent-status-change\"}",
  "content_length": 67
}

DEBUG   http    HTTP request headers {
  "destination": "analytics-api",
  "headers": {
    "Content-Type": ["application/json"],
    "Authorization": ["Bearer token"],
    "X-Event-Source": ["freeswitch-esl"],
    "X-Analytics-Version": ["v1.0"]
  }
}
```

## Validation

### Automatic validation

Khi tải config, hệ thống sẽ validate:

- `format` phải là một trong: `json`, `xml`, `form` hoặc empty (mặc định json)
- `template` không được là chuỗi trống nếu được cung cấp
- `headers` phải có key và value hợp lệ

### Test validation

```bash
# Test config validation
./fsevents config validate --config configs/your-config.yaml

# Kết quả thành công
✅ Configuration is valid!
🌐 HTTP Destinations:
  1. analytics-api -> POST https://api.analytics.com/events
  2. call-tracker -> POST https://tracker.example.com/calls
```

## Use cases thực tế

### 1. Multi-tenant Analytics

```yaml
# Gửi đến analytics với tenant ID khác nhau
- name: "tenant-a-analytics"
  url: "https://analytics.example.com/ingest"
  payload_template:
    format: "json"
    headers:
      X-Tenant-ID: "tenant-a"
      X-API-Version: "v2"

- name: "tenant-b-analytics"
  url: "https://analytics.example.com/ingest"
  payload_template:
    format: "json"
    headers:
      X-Tenant-ID: "tenant-b"
      X-API-Version: "v2"
```

### 2. Legacy System Integration

```yaml
# Hệ thống cũ yêu cầu XML
- name: "legacy-system"
  url: "https://legacy.example.com/events"
  headers:
    Content-Type: "application/xml"
  payload_template:
    format: "xml"
    headers:
      X-Legacy-Version: "1.0"
      X-Source-System: "freeswitch"

# Hệ thống mới dùng JSON
- name: "modern-api"
  url: "https://api.example.com/webhooks"
  headers:
    Content-Type: "application/json"
  payload_template:
    format: "json"
    headers:
      X-Webhook-Version: "2024-01"
```

### 3. Monitoring và Alerting

```yaml
# Alerts system - Form data
- name: "alerting-system"
  url: "https://alerts.example.com/webhook"
  headers:
    Content-Type: "application/x-www-form-urlencoded"
  event_filters: ["CHANNEL_HANGUP"]
  payload_template:
    format: "form"
    headers:
      X-Alert-Type: "call-event"
      X-Priority: "normal"

# Metrics system - JSON
- name: "metrics-collector"
  url: "https://metrics.example.com/events"
  headers:
    Content-Type: "application/json"
  event_filters: ["HEARTBEAT"]
  payload_template:
    format: "json"
    headers:
      X-Metrics-Type: "system-stats"
      X-Collection-Interval: "60s"
```

## Migration từ global template

### Trước đây (global template)

```yaml
events:
  payload_template:
    format: "json"
    headers:
      X-Global-Header: "value"

http:
  destinations:
    - name: "destination1"
      # Tất cả destinations dùng template global
    - name: "destination2"
      # Tất cả destinations dùng template global
```

### Bây giờ (per-destination templates)

```yaml
# Không còn global payload_template

http:
  destinations:
    - name: "destination1"
      payload_template:
        format: "json"
        headers:
          X-Custom-Header-1: "value1"
    
    - name: "destination2"
      payload_template:
        format: "xml"
        headers:
          X-Custom-Header-2: "value2"
    
    - name: "destination3"
      # Không có template - dùng JSON mặc định
```

## Roadmap

### Tính năng tương lai

1. **Go Template Processing**: Hỗ trợ Go template syntax trong `template` field
2. **Custom Template Functions**: Template functions cho date formatting, JSON manipulation
3. **Template Validation**: Validate template syntax khi load config
4. **Performance Optimization**: Cache compiled templates
5. **More Formats**: Hỗ trợ YAML, CSV, Protocol Buffers

### Template syntax (đã hoàn thành)

Tính năng Go template đã được triển khai đầy đủ với các template functions tùy chỉnh:

```yaml
payload_template:
  format: "json"
  headers:
    X-Custom-Header: "value"
  template: |
    {
      "event": "{{.event_name}}",
      "timestamp": "{{.timestamp}}",
      "processed_at": "{{now | formatTimeRFC3339}}",
      "data": {{.| toJSON}},
      "metadata": {
        "source": "freeswitch",
        "tenant": "{{.agent_id | default "unknown"}}"
      }
    }
```

#### Template Functions đã hỗ trợ

**Date/Time Functions:**
- `now` - Current time
- `formatTime "layout" time` - Format time with custom layout
- `formatTimeRFC3339 time` - Format time as RFC3339
- `formatTimeUnix time` - Unix timestamp (seconds)
- `formatTimeUnixMilli time` - Unix timestamp (milliseconds)

**JSON Functions:**
- `toJSON value` - Convert to JSON string
- `toJSONPretty value` - Convert to pretty JSON string

**String Functions:**
- `upper string` - Convert to uppercase
- `lower string` - Convert to lowercase
- `trim string` - Trim whitespace
- `replace "old" "new" string` - Replace all occurrences
- `contains "substr" string` - Check if contains substring
- `hasPrefix "prefix" string` - Check if has prefix
- `hasSuffix "suffix" string` - Check if has suffix

**Utility Functions:**
- `default defaultVal value` - Use default if value is empty
- `if condition trueVal falseVal` - Conditional value
- `add a b` - Add two integers
- `subtract a b` - Subtract two integers
- `multiply a b` - Multiply two integers
- `divide a b` - Divide two integers

#### Template Examples

**1. Basic JSON with custom structure:**
```yaml
template: |
  {
    "event_type": "{{.event_name | upper}}",
    "time": "{{now | formatTimeRFC3339}}",
    "agent": "{{.agent_id | default "unknown"}}",
    "action": "{{.action | upper}}"
  }
```

**2. Conditional formatting:**
```yaml
template: |
  {
    "event": "{{.event_name}}",
    {{if eq .event_name "CUSTOM"}}
    "callcenter_data": {
      "agent": "{{.agent_id}}",
      "queue": "{{.queue_name}}"
    }
    {{else if contains "CHANNEL" .event_name}}
    "call_data": {
      "call_id": "{{.call_id}}",
      "caller": "{{.caller_number}}"
    }
    {{else}}
    "raw_data": {{.| toJSON}}
    {{end}}
  }
```

**3. Slack webhook format:**
```yaml
template: |
  {
    "text": "Agent {{.agent_id}} {{.action}}",
    "blocks": [
      {
        "type": "section",
        "text": {
          "type": "mrkdwn",
          "text": "*Queue:* {{.queue_name | default "Unknown"}}\n*State:* {{.agent_state | default "Unknown"}}"
        }
      }
    ]
  }
```

**4. XML format:**
```yaml
template: |
  <?xml version="1.0" encoding="UTF-8"?>
  <event>
    <name>{{.event_name}}</name>
    <timestamp>{{.timestamp}}</timestamp>
    <agent_id>{{.agent_id | default ""}}</agent_id>
    <processed_at>{{now | formatTime "2006-01-02T15:04:05Z07:00" now}}</processed_at>
  </event>
```

**5. Form data:**
```yaml
template: |
  event={{.event_name}}&timestamp={{.timestamp}}&agent={{.agent_id | default "unknown"}}&action={{.action | default "unknown"}}&time={{now | formatTimeUnix}}
```

**6. CSV format:**
```yaml
template: |
  "{{.timestamp}}","{{.event_name}}","{{.action | default ""}}","{{.agent_id | default ""}}","{{.queue_name | default ""}}"
```

## Troubleshooting

### Lỗi thường gặp

1. **Invalid format error**
   ```
   Error: invalid payload format: txt (supported: json, xml, form)
   ```
   → Sử dụng format hợp lệ: `json`, `xml`, hoặc `form`

2. **Empty template error**
   ```
   Error: template cannot be empty string
   ```
   → Xóa `template: ""` hoặc cung cấp template hợp lệ

3. **Header validation error**
   ```
   Error: header key cannot be empty
   ```
   → Đảm bảo tất cả headers có key và value

### Performance notes

- Mỗi destination tạo payload riêng biệt (parallel processing)
- Headers từ template được merge với headers của destination
- XML và Form format hiện tại fallback về JSON (sẽ được implement đầy đủ)

## Example đầy đủ

Xem file `configs/config_per_destination_templates.yaml` cho ví dụ cấu hình đầy đủ với nhiều destinations và templates khác nhau. 