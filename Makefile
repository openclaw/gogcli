SHELL := /bin/bash

# `make` should build the binary by default.
.DEFAULT_GOAL := build

.PHONY: build gog gogcli gog-help gogcli-help help fmt fmt-check lint test ci tools
.PHONY: worker-ci

BIN_DIR := $(CURDIR)/bin
BIN := $(BIN_DIR)/gog
CMD := ./cmd/gog

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo "")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/steipete/gogcli/internal/cmd.version=$(VERSION) -X github.com/steipete/gogcli/internal/cmd.commit=$(COMMIT) -X github.com/steipete/gogcli/internal/cmd.date=$(DATE)
# `make lint` already covers vet-equivalent checks; skip duplicate work in `make test`.
GO_TEST_FLAGS ?= -vet=off
TEST_FLAGS ?=
TEST_PKGS ?= ./...

TOOLS_DIR := $(CURDIR)/.tools
GOFUMPT := $(TOOLS_DIR)/gofumpt
GOIMPORTS := $(TOOLS_DIR)/goimports
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint
TOOLS_STAMP := $(TOOLS_DIR)/.versions
TOOLS_VERSION := gofumpt=v0.9.2;goimports=v0.42.0;golangci-lint=v2.10.1

# Allow passing CLI args as extra "targets":
#   make gogcli -- --help
#   make gogcli -- gmail --help
ifneq ($(filter gogcli gog,$(MAKECMDGOALS)),)
RUN_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
$(eval $(RUN_ARGS):;@:)
endif

build:
	@mkdir -p $(BIN_DIR)
	@go build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD)

gog: build
	@if [ -n "$(RUN_ARGS)" ]; then \
		$(BIN) $(RUN_ARGS); \
	elif [ -z "$(ARGS)" ]; then \
		$(BIN) --help; \
	else \
		$(BIN) $(ARGS); \
	fi

gogcli: build
	@if [ -n "$(RUN_ARGS)" ]; then \
		$(BIN) $(RUN_ARGS); \
	elif [ -z "$(ARGS)" ]; then \
		$(BIN) --help; \
	else \
		$(BIN) $(ARGS); \
	fi

gog-help: build
	@$(BIN) --help

gogcli-help: build
	@$(BIN) --help

help: gog-help

tools:
	@mkdir -p $(TOOLS_DIR)
	@if [ -x "$(GOFUMPT)" ] && [ -x "$(GOIMPORTS)" ] && [ -x "$(GOLANGCI_LINT)" ] && [ "$$(cat $(TOOLS_STAMP) 2>/dev/null)" = "$(TOOLS_VERSION)" ]; then \
		echo "tools up to date"; \
	else \
		GOBIN=$(TOOLS_DIR) go install mvdan.cc/gofumpt@v0.9.2; \
		GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@v0.42.0; \
		GOBIN=$(TOOLS_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1; \
		printf '%s\n' "$(TOOLS_VERSION)" > "$(TOOLS_STAMP)"; \
	fi

fmt: tools
	@$(GOIMPORTS) -local github.com/steipete/gogcli -w .
	@$(GOFUMPT) -w .

fmt-check: tools
	@$(GOIMPORTS) -local github.com/steipete/gogcli -w .
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
	@go test $(GO_TEST_FLAGS) $(TEST_FLAGS) $(TEST_PKGS)

ci: pnpm-gate fmt-check lint test

worker-ci:
	@pnpm -C internal/tracking/worker lint
	@pnpm -C internal/tracking/worker build
	@pnpm -C internal/tracking/worker test
