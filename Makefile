BINARY      := dynamoctl
MODULE      := github.com/ffreis/dynamoctl
CMD_PKG     := ./cmd/$(BINARY)
GO          ?= $(shell command -v go 2>/dev/null || echo /usr/local/go/bin/go)
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -ldflags "-X '$(MODULE)/cmd.version=$(VERSION)' \
                          -X '$(MODULE)/cmd.commit=$(COMMIT)' \
                          -X '$(MODULE)/cmd.buildTime=$(BUILD_TIME)'"
COVERAGE_THRESHOLD := 80
GOTEST      := $(GO) test -timeout 60s -race -shuffle=on

.PHONY: all build test lint fmt fmt-check tidy coverage coverage-gate \
        lefthook-install clean smoke help

all: fmt-check lint test build  ## Run all quality gates and build

## ── Build ──────────────────────────────────────────────────────────────────

build:  ## Build the binary
	$(GO) build -trimpath $(LDFLAGS) -o $(BINARY) $(CMD_PKG)

## ── Testing ────────────────────────────────────────────────────────────────

test:  ## Run all unit tests
	$(GOTEST) ./...

coverage:  ## Generate HTML coverage report
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

coverage-gate:  ## Fail if total coverage is below $(COVERAGE_THRESHOLD)%
	$(GOTEST) -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tee /dev/stderr | \
		awk '/^total:/ { gsub(/%/, "", $$3); if ($$3 < $(COVERAGE_THRESHOLD)) \
		{ print "Coverage " $$3 "% is below threshold $(COVERAGE_THRESHOLD)%"; exit 1 } }'

## ── Code quality ───────────────────────────────────────────────────────────

lint:  ## Run golangci-lint
	golangci-lint run ./...

fmt:  ## Format all Go source files
	gofmt -w -s .
	goimports -w .

fmt-check:  ## Fail if any file would be reformatted
	@out=$$(gofmt -l -s .); \
	if [ -n "$$out" ]; then \
		echo "Unformatted files:"; echo "$$out"; exit 1; \
	fi

tidy:  ## Tidy and verify go.mod / go.sum
	$(GO) mod tidy
	$(GO) mod verify

## ── Hooks ──────────────────────────────────────────────────────────────────

lefthook-install:  ## Install git hooks via lefthook
	lefthook install

## ── Maintenance ────────────────────────────────────────────────────────────

clean:  ## Remove build artefacts
	rm -f $(BINARY) coverage.out coverage.html

help:  ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

PLATFORM_STANDARDS_SHA ?= 3c787edb4e96ddea2e86b2add2c32139685e8db7  # v1.2.1
PLATFORM_STANDARDS_RAW ?= https://raw.githubusercontent.com/FelipeFuhr/ffreis-platform-standards

install-act: ## Download pinned act binary into .bin/
	@mkdir -p scripts
	@curl -fsSL "$(PLATFORM_STANDARDS_RAW)/$(PLATFORM_STANDARDS_SHA)/scripts/install_act.sh" \
		-o scripts/install_act.sh && chmod +x scripts/install_act.sh
	@bash ./scripts/install_act.sh

ci-local: ## Run workflows locally via act (GH Actions quota fallback). Args via ARGS=...
	@mkdir -p scripts
	@curl -fsSL "$(PLATFORM_STANDARDS_RAW)/$(PLATFORM_STANDARDS_SHA)/scripts/run-ci-local.sh" \
		-o scripts/run-ci-local.sh && chmod +x scripts/run-ci-local.sh
	@PATH="$(CURDIR)/.bin:$(PATH)" bash ./scripts/run-ci-local.sh $(ARGS)

# --- lefthook (simple set on commit; complex/release via /ready + manual CI) ---
LEFTHOOK_VERSION ?= 1.7.10
LEFTHOOK_DIR ?= $(CURDIR)/.bin
LEFTHOOK_BIN ?= $(LEFTHOOK_DIR)/lefthook
secrets-scan-staged:
	@command -v gitleaks >/dev/null 2>&1 && gitleaks protect --staged --redact || echo "gitleaks not installed; skipping"
lefthook-bootstrap:
	LEFTHOOK_VERSION="$(LEFTHOOK_VERSION)" BIN_DIR="$(LEFTHOOK_DIR)" bash ./scripts/bootstrap_lefthook.sh
lefthook-install: lefthook-bootstrap
	LEFTHOOK="$(LEFTHOOK_BIN)" "$(LEFTHOOK_BIN)" install
setup: lefthook-install
	@echo "lefthook hooks installed"
