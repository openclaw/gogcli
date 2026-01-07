SHELL := /bin/bash

# `make` should build the binary by default.
.DEFAULT_GOAL := build

.PHONY: build gog gogcli gog-help gogcli-help help fmt fmt-check lint test ci tools

BIN_DIR := $(CURDIR)/bin
BIN := $(BIN_DIR)/gog
CMD := ./cmd/gog

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo "")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/steipete/gogcli/internal/cmd.version=$(VERSION) -X github.com/steipete/gogcli/internal/cmd.commit=$(COMMIT) -X github.com/steipete/gogcli/internal/cmd.date=$(DATE)

TOOLS_DIR := $(CURDIR)/.tools
GOFUMPT := $(TOOLS_DIR)/gofumpt
GOIMPORTS := $(TOOLS_DIR)/goimports
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint

# Allow passing CLI args as extra "targets":
#   make gogcli -- --help
#   make gogcli -- gmail --help
ifneq ($(filter gogcli,$(MAKECMDGOALS)),)
RUN_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
$(eval $(RUN_ARGS):;@:)
endif

build:
	@mkdir -p $(BIN_DIR)
	@go build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD)

gog: build
	@if [ -z "$(ARGS)" ]; then \
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
	@GOBIN=$(TOOLS_DIR) go install mvdan.cc/gofumpt@v0.9.2
	@GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@v0.40.0
	@GOBIN=$(TOOLS_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6

fmt: tools
	@$(GOIMPORTS) -local github.com/steipete/gogcli -w .
	@$(GOFUMPT) -w .

fmt-check: tools
	@$(GOIMPORTS) -local github.com/steipete/gogcli -w .
	@$(GOFUMPT) -w .
	@git diff --exit-code -- '*.go' go.mod go.sum

lint: tools
	@$(GOLANGCI_LINT) run

test:
	@go test ./...

ci: fmt-check lint test
