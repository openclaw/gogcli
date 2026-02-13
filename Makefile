SHELL := /bin/bash

# `make` should build the binary by default.
.DEFAULT_GOAL := build

.PHONY: build rata ratatosk rata-help ratatosk-help help fmt fmt-check lint test ci tools
.PHONY: worker-ci

BIN_DIR := $(CURDIR)/bin
BIN := $(BIN_DIR)/rata
CMD := ./cmd/rata

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo "")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/degree-analytics/ratatosk/internal/cmd.version=$(VERSION) -X github.com/degree-analytics/ratatosk/internal/cmd.commit=$(COMMIT) -X github.com/degree-analytics/ratatosk/internal/cmd.date=$(DATE)

TOOLS_DIR := $(CURDIR)/.tools
GOFUMPT := $(TOOLS_DIR)/gofumpt
GOIMPORTS := $(TOOLS_DIR)/goimports
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint

# Allow passing CLI args as extra "targets":
#   make ratatosk -- --help
#   make ratatosk -- gmail --help
ifneq ($(filter ratatosk rata,$(MAKECMDGOALS)),)
RUN_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
$(eval $(RUN_ARGS):;@:)
endif

build:
	@mkdir -p $(BIN_DIR)
	@go build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD)

rata: build
	@if [ -n "$(RUN_ARGS)" ]; then \
		$(BIN) $(RUN_ARGS); \
	elif [ -z "$(ARGS)" ]; then \
		$(BIN) --help; \
	else \
		$(BIN) $(ARGS); \
	fi

ratatosk: build
	@if [ -n "$(RUN_ARGS)" ]; then \
		$(BIN) $(RUN_ARGS); \
	elif [ -z "$(ARGS)" ]; then \
		$(BIN) --help; \
	else \
		$(BIN) $(ARGS); \
	fi

rata-help: build
	@$(BIN) --help

ratatosk-help: build
	@$(BIN) --help

help: rata-help

tools:
	@mkdir -p $(TOOLS_DIR)
	@GOBIN=$(TOOLS_DIR) go install mvdan.cc/gofumpt@v0.9.2
	@GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@v0.41.0
	@GOBIN=$(TOOLS_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0

fmt: tools
	@$(GOIMPORTS) -local github.com/degree-analytics/ratatosk -w .
	@$(GOFUMPT) -w .

fmt-check: tools
	@$(GOIMPORTS) -local github.com/degree-analytics/ratatosk -w .
	@$(GOFUMPT) -w .
	@git diff --exit-code -- '*.go' go.mod go.sum

lint: tools
	@$(GOLANGCI_LINT) run

pnpm-gate:
	@if [ -f package.json ] || [ -f package.json5 ] || [ -f package.yaml ]; then \
		pnpm lint && pnpm build && pnpm test; \
	else \
		echo "pnpm gate skipped (no package.json)"; \
	fi

test:
	@go test ./...

ci: pnpm-gate fmt-check lint test

worker-ci:
	@pnpm -C internal/tracking/worker lint
	@pnpm -C internal/tracking/worker build
	@pnpm -C internal/tracking/worker test
