# =============================================================
# vpc-proof-agent: Makefile
# Build, test, lint, format, run, and LocalStack automation.
# =============================================================

SHELL := /usr/bin/env bash
BINARY     := bin/vpc-proof
GOBIN      := $(CURDIR)/bin
MOCKGEN    := $(GOBIN)/mockgen

GO         ?= go
GOLANGCI   ?= golangci-lint
GORELEASER ?= goreleaser

# --- Build metadata (injected into internal/buildinfo via -ldflags) -----------

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

BUILDINFO_PKG := github.com/emanuellcs/vpc-proof-agent/internal/buildinfo
LDFLAGS := -s -w \
	-X $(BUILDINFO_PKG).Version=$(VERSION) \
	-X $(BUILDINFO_PKG).Commit=$(COMMIT) \
	-X $(BUILDINFO_PKG).BuildDate=$(BUILD_DATE) \
	-X '$(BUILDINFO_PKG).Developer=Emanuel Lázaro (emanuellcs)'

# --- Default target ---------------------------------------------------------

.PHONY: help
help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# --- Build & run ------------------------------------------------------------

.PHONY: build
build: ## Build the binary into bin/vpc-proof (with version ldflags)
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/vpc-proof

.PHONY: run
run: ## Run the vpc-proof binary
	$(GO) run ./cmd/vpc-proof

.PHONY: install
install: ## Install the binary to GOBIN (with version ldflags)
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/vpc-proof

# --- Test & static analysis -------------------------------------------------

.PHONY: test
test: ## Run all tests with the race detector
	$(GO) test -race -count=1 ./...

.PHONY: e2e
e2e: ## Run end-to-end tests against the compiled binary
	$(GO) test -tags e2e -count=1 -v ./test/e2e/

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format code (gofumpt + goimports via golangci-lint)
	$(GOLANGCI) fmt

.PHONY: lint
lint: ## Run golangci-lint
	$(GOLANGCI) run

.PHONY: tidy
tidy: ## Tidy Go modules
	$(GO) mod tidy

# --- Tooling -----------------------------------------------------------------

.PHONY: tools
tools: ## Install development tools (mockgen, goreleaser)
	@mkdir -p bin
	GOBIN=$(GOBIN) $(GO) install go.uber.org/mock/mockgen@latest
	GOBIN=$(GOBIN) $(GO) install github.com/goreleaser/goreleaser/v2@latest

.PHONY: mocks
mocks: ## Generate mocks via go:generate directives
	$(GO) generate ./...

# --- LocalStack ----------------------------------------------------------------

.PHONY: run-status
run-status: build ## Quick instance status (graceful under LocalStack)
	$(BINARY) status

.PHONY: run-check
run-check: build ## Run the full probe suite (exit code is a CI gateway)
	$(BINARY) check

.PHONY: run-report
run-report: build ## Generate a Markdown evidence report to stdout
	$(BINARY) report --format markdown

.PHONY: localstack-setup
localstack-setup: ## Provision the AWS lab inside LocalStack
	./scripts/setup-localstack.sh

.PHONY: localstack-teardown
localstack-teardown: ## Tear down the AWS lab from LocalStack
	./scripts/teardown-localstack.sh

# --- Release automation --------------------------------------------------------

.PHONY: release-dry-run
release-dry-run: ## Test packaging locally with GoReleaser (snapshot, no publish)
	$(GORELEASER) release --snapshot --clean

# --- Cleanup ------------------------------------------------------------------

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin dist build
