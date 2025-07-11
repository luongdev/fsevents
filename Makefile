# FreeSWITCH ESL Sidecar Makefile

# Variables
APP_NAME := fsevents
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
REGISTRY ?= 
IMAGE_TAG := $(if $(REGISTRY),$(REGISTRY)/$(APP_NAME),$(APP_NAME))

# Go variables
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
CGO_ENABLED ?= 0

# Build flags
LDFLAGS := -w -s -X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: help
help: ## Show this help message
	@echo "FreeSWITCH ESL Sidecar - Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the application binary
	@echo "Building $(APP_NAME) $(VERSION)..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-ldflags="$(LDFLAGS)" \
		-o $(APP_NAME) \
		cmd/fsevents/main.go
	@echo "✅ Binary built: $(APP_NAME)"

.PHONY: build-linux
build-linux: ## Build Linux binary
	$(MAKE) build GOOS=linux GOARCH=amd64

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -f $(APP_NAME)
	@rm -rf dist/
	@echo "✅ Clean completed"

.PHONY: test
test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

.PHONY: lint
lint: ## Run linter
	@echo "Running linter..."
	golangci-lint run

.PHONY: fmt
fmt: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...

.PHONY: mod-tidy
mod-tidy: ## Tidy Go modules
	@echo "Tidying Go modules..."
	go mod tidy

# Docker targets
.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image $(IMAGE_TAG):$(VERSION)..."
	./scripts/build-docker.sh $(VERSION)

.PHONY: docker-build-latest
docker-build-latest: ## Build Docker image with latest tag
	@echo "Building Docker image $(IMAGE_TAG):latest..."
	./scripts/build-docker.sh latest

.PHONY: docker-run
docker-run: ## Run Docker container
	@echo "Running Docker container..."
	docker run -d \
		--name $(APP_NAME)-sidecar \
		-p 9090:9090 \
		-v $(PWD)/configs:/app/configs:ro \
		-v $(PWD)/logs:/app/logs \
		$(IMAGE_TAG):latest

.PHONY: docker-stop
docker-stop: ## Stop Docker container
	@echo "Stopping Docker container..."
	-docker stop $(APP_NAME)-sidecar
	-docker rm $(APP_NAME)-sidecar

.PHONY: docker-logs
docker-logs: ## Show Docker container logs
	docker logs -f $(APP_NAME)-sidecar

.PHONY: docker-shell
docker-shell: ## Open shell in Docker container
	docker exec -it $(APP_NAME)-sidecar /bin/sh

.PHONY: docker-push
docker-push: ## Push Docker image to registry
	@if [ -z "$(REGISTRY)" ]; then \
		echo "❌ REGISTRY not set. Use: make docker-push REGISTRY=your-registry.com"; \
		exit 1; \
	fi
	@echo "Pushing $(IMAGE_TAG):$(VERSION) to registry..."
	docker push $(IMAGE_TAG):$(VERSION)
	docker push $(IMAGE_TAG):latest
	@echo "✅ Images pushed to registry"

.PHONY: docker-clean
docker-clean: ## Clean Docker images and containers
	@echo "Cleaning Docker artifacts..."
	-docker stop $(APP_NAME)-sidecar
	-docker rm $(APP_NAME)-sidecar
	-docker rmi $(APP_NAME):latest $(APP_NAME):$(VERSION)
	@echo "✅ Docker cleanup completed"

# Docker Compose targets
.PHONY: compose-up
compose-up: ## Start services with docker-compose
	@echo "Starting services with docker-compose..."
	docker-compose up -d

.PHONY: compose-down
compose-down: ## Stop services with docker-compose
	@echo "Stopping services with docker-compose..."
	docker-compose down

.PHONY: compose-logs
compose-logs: ## Show docker-compose logs
	docker-compose logs -f

.PHONY: compose-build
compose-build: ## Build with docker-compose
	@echo "Building with docker-compose..."
	docker-compose build

# Development targets
.PHONY: dev
dev: ## Run in development mode
	@echo "Starting development server..."
	go run cmd/fsevents/main.go --config configs/config.yaml --log-level debug

.PHONY: install-deps
install-deps: ## Install development dependencies
	@echo "Installing development dependencies..."
	go mod download
	@echo "✅ Dependencies installed"

.PHONY: check
check: fmt mod-tidy test lint ## Run all checks (format, test, lint)

# Release targets
.PHONY: release-build
release-build: clean ## Build release binaries for multiple platforms
	@echo "Building release binaries..."
	@mkdir -p dist
	# Linux AMD64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="$(LDFLAGS)" \
		-o dist/$(APP_NAME)-linux-amd64 \
		cmd/fsevents/main.go
	# Linux ARM64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-ldflags="$(LDFLAGS)" \
		-o dist/$(APP_NAME)-linux-arm64 \
		cmd/fsevents/main.go
	# macOS AMD64
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-ldflags="$(LDFLAGS)" \
		-o dist/$(APP_NAME)-darwin-amd64 \
		cmd/fsevents/main.go
	# macOS ARM64
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-ldflags="$(LDFLAGS)" \
		-o dist/$(APP_NAME)-darwin-arm64 \
		cmd/fsevents/main.go
	@echo "✅ Release binaries built in dist/"

.PHONY: package
package: release-build ## Package release binaries
	@echo "Packaging release binaries..."
	@cd dist && \
	for binary in $(APP_NAME)-*; do \
		tar -czf $$binary.tar.gz $$binary; \
		echo "Packaged: $$binary.tar.gz"; \
	done
	@echo "✅ Release packages created"

# Info targets
.PHONY: info
info: ## Show build information
	@echo "Build Information:"
	@echo "  App Name:    $(APP_NAME)"
	@echo "  Version:     $(VERSION)"
	@echo "  Git Commit:  $(GIT_COMMIT)"
	@echo "  Build Date:  $(BUILD_DATE)"
	@echo "  Go Version:  $(shell go version)"
	@echo "  Platform:    $(GOOS)/$(GOARCH)"
	@echo "  Registry:    $(if $(REGISTRY),$(REGISTRY),not set)"

.PHONY: docker-info
docker-info: ## Show Docker image information
	@echo "Docker Image Information:"
	@docker images $(APP_NAME) --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedAt}}"

# Default target
.DEFAULT_GOAL := help 