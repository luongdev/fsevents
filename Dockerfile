# Build stage
FROM golang:1.24-alpine AS builder

# Install git for downloading dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o fsevents \
    cmd/fsevents/main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S fsevents && \
    adduser -u 1001 -S fsevents -G fsevents

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/fsevents .

# Copy configuration files
COPY --from=builder /app/configs ./configs

# Create logs directory
RUN mkdir -p logs && chown -R fsevents:fsevents /app

# Switch to non-root user
USER fsevents

# Default command
CMD ["./fsevents", "--config", "configs/config.yaml"] 