# FreeSWITCH ESL Sidecar App

A Go-based sidecar application that connects to FreeSWITCH Event Socket Library (ESL) to receive events and forward them to HTTP endpoints.

## Features

- Connect to FreeSWITCH ESL
- Receive and process events
- Forward events to HTTP destinations
- Configurable event filtering
- Retry mechanisms
- Monitoring and metrics

## Project Structure

```
fsevents/
├── cmd/fsevents/          # Application entry point
├── internal/              # Internal packages
│   ├── config/           # Configuration management
│   ├── esl/              # ESL client
│   ├── processor/        # Event processing
│   ├── http/             # HTTP client
│   ├── logger/           # Logging
│   └── metrics/          # Metrics collection
├── pkg/types/            # Public types
├── configs/              # Configuration files
└── deployments/          # Deployment configs
```

## Quick Start

```bash
# Build the application
go build -o fsevents cmd/fsevents/main.go

# Run the application
./fsevents

# Check version
./fsevents version
```

## Development Status

This project is under active development. See CHECKLIST.md for implementation progress. 