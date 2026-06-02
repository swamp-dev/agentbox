.PHONY: build test test-unit test-integration test-coverage clean install lint fmt smoke docker-build mutation help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-X github.com/swamp-dev/agentbox/internal/cli.Version=$(VERSION) -X github.com/swamp-dev/agentbox/internal/cli.Commit=$(COMMIT) -X github.com/swamp-dev/agentbox/internal/cli.BuildDate=$(BUILD_DATE)"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the agentbox binary
	go build $(LDFLAGS) -o bin/agentbox ./cmd/agentbox

install: build ## Install agentbox to $GOPATH/bin
	go install $(LDFLAGS) ./cmd/agentbox

test: ## Run tests with race detector
	go test -race -v ./...

test-unit: ## Run unit tests only (no Docker required)
	go test -race -short -v ./...

test-integration: ## Run integration tests (requires Docker)
	go test -race -v -run 'TestRun|TestContainer' ./internal/container/

test-coverage: ## Run tests with race detector and coverage report
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
	@go tool cover -func=coverage.out | grep total

ci: ## Run full CI suite locally (mirrors GitHub Actions: lint + race-test + coverage ≥ 65%)
	@$(MAKE) lint
	go test -race -coverprofile=coverage.out ./...
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Coverage: $${COVERAGE}%"; \
	if [ "$$(awk "BEGIN {print ($${COVERAGE} < 65)}")" -eq 1 ]; then \
	  echo "Coverage $${COVERAGE}% is below 65% threshold"; exit 1; \
	fi

lint: ## Run linters
	golangci-lint run ./...

fmt: ## Format code
	go fmt ./...
	goimports -w .

smoke: build ## Smoke test: run help and version commands
	bin/agentbox --help
	bin/agentbox version

mutation: ## Run mutation testing on core packages (slow — ~5 min per package; requires gremlins)
	@command -v gremlins >/dev/null 2>&1 || { echo "gremlins not installed. Run: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest"; exit 1; }
	gremlins unleash --coverpkg ./internal/ralph/... ./internal/ralph/
	gremlins unleash --coverpkg ./internal/supervisor/... ./internal/supervisor/
	gremlins unleash --coverpkg ./internal/store/... ./internal/store/

clean: ## Clean build artifacts
	rm -rf bin/
	rm -f coverage.out coverage.html

docker-build: ## Build Docker images locally
	docker build -t agentbox/base:latest -f images/base/Dockerfile .
	docker build -t agentbox/node:20 -f images/node/Dockerfile .
	docker build -t agentbox/python:3.12 -f images/python/Dockerfile .
	docker build -t agentbox/go:1.24 -f images/go/Dockerfile .
	docker build -t agentbox/rust:1.77 -f images/rust/Dockerfile .
	docker build -t agentbox/full:latest -f images/full/Dockerfile .

docker-build-full: ## Build only the full Docker image
	docker build -t agentbox/full:latest -f images/full/Dockerfile .

release: ## Build release binaries for all platforms
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/agentbox-darwin-amd64 ./cmd/agentbox
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/agentbox-darwin-arm64 ./cmd/agentbox
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/agentbox-linux-amd64 ./cmd/agentbox
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/agentbox-linux-arm64 ./cmd/agentbox
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/agentbox-windows-amd64.exe ./cmd/agentbox

.DEFAULT_GOAL := help
