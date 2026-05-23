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
