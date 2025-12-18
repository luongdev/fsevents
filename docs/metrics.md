# Metrics Server

The FSEvents application includes a built-in HTTP metrics server that exposes application metrics and health information.

## Configuration

Configure the metrics server in your `config.yaml`:

```yaml
metrics:
  enabled: true    # Enable/disable metrics server
  port: 9090      # HTTP port to listen on
  path: "/metrics" # Metrics endpoint path
```

## Endpoints

### GET /metrics

Returns comprehensive application metrics in JSON format.

**Example Response:**
```json
{
  "start_time": "2024-01-15T10:30:00Z",
  "uptime": "2h15m30s",
  "esl_connected": true,
  "esl_connected_at": "2024-01-15T10:30:05Z",
  "esl_events_received": 15234,
  "esl_reconnect_attempts": 0,
  "events_received": 15234,
  "events_processed": 15230,
  "events_dropped": 4,
  "queue_depth": 12,
  "queue_capacity": 10000,
  "queue_usage_percent": 0.12,
  "active_workers": 3,
  "total_workers": 8,
  "events_per_second": 2.1
}
```

**Metrics Description:**

| Metric | Description |
|--------|-------------|
| `start_time` | Application start timestamp |
| `uptime` | Application uptime duration |
| `esl_connected` | ESL connection status |
| `esl_connected_at` | ESL connection timestamp |
| `esl_events_received` | Total events received from ESL |
| `esl_reconnect_attempts` | Number of ESL reconnection attempts |
| `events_received` | Total events received by worker pool |
| `events_processed` | Total events processed successfully |
| `events_dropped` | Total events dropped (queue full) |
| `queue_depth` | Current event queue depth |
| `queue_capacity` | Maximum event queue capacity |
| `queue_usage_percent` | Queue usage percentage |
| `active_workers` | Currently active worker goroutines |
| `total_workers` | Total worker goroutines |
| `events_per_second` | Average events processed per second |

### GET /health

Returns basic health check information.

**Example Response:**
```json
{
  "status": "ok",
  "timestamp": "2024-01-15T12:45:30Z",
  "uptime": "2h15m30s",
  "esl_connected": true
}
```

## Monitoring

### Key Metrics to Monitor

1. **Queue Usage**: `queue_usage_percent` should stay below 80%
   - If consistently high: increase `event_buffer_size` or `worker_count`

2. **Events Dropped**: `events_dropped` should be 0 or very low
   - If high: increase buffer size or worker count

3. **ESL Connection**: `esl_connected` should be `true`
   - Monitor `esl_reconnect_attempts` for connection stability

4. **Processing Rate**: `events_per_second` indicates throughput
   - Compare with `esl_events_received` rate

### Alerting Recommendations

- Alert if `queue_usage_percent > 80`
- Alert if `events_dropped > 0` (sustained)
- Alert if `esl_connected = false`
- Alert if `events_per_second` drops significantly

## Integration

### Prometheus

The metrics endpoint returns JSON format. For Prometheus integration, you can use a JSON exporter or create a custom exporter.

### Grafana Dashboard

Create dashboards using the metrics data:

1. **System Overview**: uptime, connection status
2. **Event Processing**: events/sec, queue usage, drops
3. **Worker Pool**: active workers, queue depth
4. **ESL Connection**: connection status, reconnects

### Health Checks

Use the `/health` endpoint for:
- Load balancer health checks
- Container orchestration health probes
- Monitoring system health checks

## Troubleshooting

### Metrics Server Not Starting

1. Check if port is already in use:
   ```bash
   lsof -i :9090
   ```

2. Verify configuration:
   ```bash
   curl http://localhost:9090/health
   ```

3. Check application logs for metrics server errors

### High Queue Usage

If `queue_usage_percent` is consistently high:

1. Increase `event_buffer_size` in config
2. Increase `worker_count` 
3. Optimize HTTP endpoint response times
4. Enable event filtering to reduce load

### Events Being Dropped

If `events_dropped > 0`:

1. Increase `event_buffer_size`
2. Increase `worker_count`
3. Check HTTP endpoint performance
4. Monitor network connectivity