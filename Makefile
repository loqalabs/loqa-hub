.PHONY: build test lint lint-fix fmt vet clean run install-tools help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=loqa-hub
BINARY_PATH=./bin/$(BINARY_NAME)

# Build targets
build: ## Build the application
	$(GOBUILD) -o $(BINARY_PATH) ./cmd

run: build ## Build and run the application
	$(BINARY_PATH)

test: ## Run tests
	$(GOTEST) -v ./...

test-coverage: ## Run tests with coverage
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

# Linting and formatting
lint: ## Run golangci-lint (includes staticcheck, errcheck, govet, etc.)
	golangci-lint run --config .golangci.yml

lint-fix: ## Run golangci-lint with auto-fix
	golangci-lint run --config .golangci.yml --fix

lint-fast: ## Run golangci-lint with only fast linters (for CI)
	golangci-lint run --config .golangci.yml --fast

fmt: ## Format code with gofmt
	gofmt -s -w .

vet: ## Run go vet
	$(GOCMD) vet ./...

# Development helpers
tidy: ## Tidy go modules
	$(GOMOD) tidy

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf $(BINARY_PATH)
	rm -rf coverage.out

# Install development tools
install-tools: ## Install development tools
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Pre-commit checks (run before committing)
pre-commit: fmt vet test lint-fast ## Run all pre-commit checks

# Quality validation without auto-formatting (catches all issues)
quality-check-strict: vet test lint ## Run comprehensive quality checks without auto-formatting

# Complete quality validation (run before pushing)
quality-check: quality-check-strict fmt ## Run quality checks, then auto-fix formatting

# Help
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)