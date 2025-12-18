#!/bin/bash

# Build script for FreeSWITCH ESL Sidecar App

set -e

# Variables
APP_NAME="fsevents"
VERSION="${VERSION:-0.1.1}"
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S_UTC')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS="-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}"

echo "Building ${APP_NAME}..."
echo "Version: ${VERSION}"
echo "Build Time: ${BUILD_TIME}"
echo "Git Commit: ${GIT_COMMIT}"

# Build for current platform
go build -ldflags "${LDFLAGS}" -o ${APP_NAME} cmd/fsevents/main.go

echo "Build completed successfully: ./${APP_NAME}"

# Make executable
chmod +x ${APP_NAME}

echo "Build information:"
./${APP_NAME} version 