# Docker Deployment Guide

This guide explains how to build and deploy the FreeSWITCH ESL Sidecar application using Docker.

## Quick Start

### Build Image

```bash
# Build with default settings
./scripts/build-docker.sh

# Build with specific version
./scripts/build-docker.sh v1.0.0

# Build and push to registry
DOCKER_REGISTRY=your-registry.com PUSH_IMAGE=true ./scripts/build-docker.sh v1.0.0
```

### Run Container

```bash
# Run with docker command
docker run -d \
  --name fsevents-sidecar \
  -p 9090:9090 \
  -v $(pwd)/configs:/app/configs:ro \
  -v $(pwd)/logs:/app/logs \
  fsevents:latest

# Run with docker-compose
docker-compose up -d
```

## Docker Configuration

### Dockerfile Features

- **Multi-stage build**: Optimized image size with separate build and runtime stages
- **Non-root user**: Runs as `fsevents` user (UID 1001) for security
- **Static binary**: CGO disabled for maximum portability
- **Health check**: Built-in health monitoring
- **Alpine base**: Small, secure base image

### Image Size

The final image is typically **~15-20MB** including:
- Alpine Linux base (~5MB)
- Static Go binary (~8-12MB)
- CA certificates and timezone data
- Configuration files

### Build Arguments

```dockerfile
ARG BUILD_DATE     # Build timestamp
ARG GIT_COMMIT     # Git commit hash  
ARG VERSION        # Application version
```

## Docker Compose

### Basic Setup

```yaml
version: '3.8'
services:
  fsevents:
    image: fsevents:latest
    container_name: fsevents-sidecar
    restart: unless-stopped
    ports:
      - "9090:9090"
    volumes:
      - ./configs:/app/configs:ro
      - ./logs:/app/logs
```

### Production Setup

```yaml
version: '3.8'
services:
  fsevents:
    image: fsevents:latest
    container_name: fsevents-sidecar
    restart: unless-stopped
    
    environment:
      - LOG_LEVEL=info
      - LOG_FORMAT=json
    
    ports:
      - "9090:9090"
    
    volumes:
      - ./configs:/app/configs:ro
      - ./logs:/app/logs
      - /etc/localtime:/etc/localtime:ro
    
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:9090/metrics"]
      interval: 30s
      timeout: 10s
      retries: 3
    
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
        reservations:
          cpus: '0.5'
          memory: 256M
    
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | info |
| `LOG_FORMAT` | Log format (json, console) | console |

## Volumes

### Required Volumes

- `/app/configs` - Configuration files (read-only)
- `/app/logs` - Log files (read-write)

### Configuration Volume

Mount your configuration directory:

```bash
-v $(pwd)/configs:/app/configs:ro
```

Ensure your config file is at `configs/config.yaml` or specify a different path:

```bash
docker run fsevents:latest ./fsevents --config configs/custom-config.yaml
```

### Logs Volume

Mount logs directory for persistence:

```bash
-v $(pwd)/logs:/app/logs
```

## Networking

### Ports

- `9090`: Metrics endpoint (if enabled)
- No other ports are exposed by default

### FreeSWITCH Connection

The container needs network access to your FreeSWITCH server. Configure in your `config.yaml`:

```yaml
esl:
  host: "your-freeswitch-server"  # Use actual hostname/IP
  port: 8021
  password: "ClueCon"
```

### HTTP Destinations

Ensure webhook destinations are accessible from the container:

```yaml
http:
  destinations:
    - name: "webhook"
      url: "http://your-webhook-server:8080/webhook"
```

## Security

### Non-Root User

The container runs as user `fsevents` (UID 1001) for security:

```dockerfile
USER fsevents
```

### Read-Only Configuration

Mount configs as read-only:

```bash
-v $(pwd)/configs:/app/configs:ro
```

### Network Security

- Only expose necessary ports
- Use internal Docker networks for service communication
- Consider using secrets management for sensitive configuration

## Health Checks

### Built-in Health Check

The Dockerfile includes a health check:

```dockerfile
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --spider http://localhost:9090/metrics || exit 1
```

### Manual Health Check

```bash
# Check container health
docker ps
docker logs fsevents-sidecar

# Check metrics endpoint
curl http://localhost:9090/metrics
```

## Monitoring

### Metrics

If metrics are enabled, they're available at:
- `http://localhost:9090/metrics` (Prometheus format)

### Logs

Application logs are written to:
- Container: `/app/logs/`
- Host: `./logs/` (if volume mounted)

View logs:

```bash
# Container logs
docker logs fsevents-sidecar

# Application logs
docker exec fsevents-sidecar tail -f logs/fsevents.log
```

## Troubleshooting

### Common Issues

1. **Config file not found**
   ```bash
   # Ensure config volume is mounted correctly
   docker exec fsevents-sidecar ls -la /app/configs/
   ```

2. **Permission issues with logs**
   ```bash
   # Fix log directory permissions
   sudo chown -R 1001:1001 logs/
   ```

3. **Network connectivity**
   ```bash
   # Test FreeSWITCH connectivity from container
   docker exec fsevents-sidecar wget -qO- --timeout=5 your-freeswitch:8021 || echo "Connection failed"
   ```

4. **Health check failing**
   ```bash
   # Check if metrics port is accessible
   docker exec fsevents-sidecar wget --spider http://localhost:9090/metrics
   ```

### Debug Mode

Run container in debug mode:

```bash
docker run -it --rm \
  -v $(pwd)/configs:/app/configs:ro \
  fsevents:latest \
  ./fsevents --config configs/config.yaml --log-level debug
```

## Registry Deployment

### Build and Push

```bash
# Set registry
export DOCKER_REGISTRY=your-registry.com/fsevents

# Build and push
PUSH_IMAGE=true ./scripts/build-docker.sh v1.0.0
```

### Pull and Run

```bash
# Pull from registry
docker pull your-registry.com/fsevents/fsevents:v1.0.0

# Run
docker run -d \
  --name fsevents-sidecar \
  -p 9090:9090 \
  -v $(pwd)/configs:/app/configs:ro \
  your-registry.com/fsevents/fsevents:v1.0.0
```

## Production Considerations

### Resource Limits

Set appropriate resource limits:

```yaml
deploy:
  resources:
    limits:
      cpus: '1.0'
      memory: 512M
    reservations:
      cpus: '0.5'
      memory: 256M
```

### Log Rotation

Configure log rotation:

```yaml
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
```

### Restart Policy

Use appropriate restart policy:

```yaml
restart: unless-stopped
```

### Secrets Management

For production, consider using Docker secrets or external secret management:

```bash
# Example with Docker secrets
echo "your-freeswitch-password" | docker secret create fs_password -
``` 