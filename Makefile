.PHONY: build test clean install lint fmt docker-build help

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

test: ## Run tests
	go test -v ./...

test-coverage: ## Run tests with coverage
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint: ## Run linters
	golangci-lint run ./...

fmt: ## Format code
	go fmt ./...
	goimports -w .

clean: ## Clean build artifacts
	rm -rf bin/
	rm -f coverage.out coverage.html

docker-build: ## Build Docker images locally
	docker build -t agentbox/base:latest -f images/base/Dockerfile .
	docker build -t agentbox/node:20 -f images/node/Dockerfile .
	docker build -t agentbox/python:3.12 -f images/python/Dockerfile .
	docker build -t agentbox/go:1.22 -f images/go/Dockerfile .
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
