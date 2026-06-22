SHELL := /bin/bash

# `make` should build the binary by default.
.DEFAULT_GOAL := build

.PHONY: build build-safe gog gogcli gog-help gogcli-help help fmt fmt-check lint deadcode test ci tools pnpm-gate docs-commands docs-site docs-check agent-skills agent-skills-check
.PHONY: worker-ci eval-gws eval-gws-agents eval-gws-test

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
DEADCODE := $(TOOLS_DIR)/deadcode
TOOLS_STAMP := $(TOOLS_DIR)/.versions
TOOLS_VERSION := gofumpt=v0.9.2;goimports=v0.44.0;golangci-lint=v2.11.4;deadcode=v0.45.0

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

build-safe:
	@./build-safe.sh $${PROFILE:-safety-profiles/agent-safe.yaml} -o $${OUTPUT:-$(BIN_DIR)/gog-safe}

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

docs-commands: build
	@scripts/gen-command-reference.sh docs/commands.generated.md

docs-site: docs-commands
	@node scripts/build-docs-site.mjs

docs-check: docs-site
	@node --test scripts/check-docs-coverage.test.mjs
	@node scripts/check-docs-coverage.mjs

agent-skills: build
	@node scripts/gen-agent-skills.mjs

agent-skills-check: build
	@node scripts/gen-agent-skills.mjs --check

tools:
	@mkdir -p $(TOOLS_DIR)
	@if [ -x "$(GOFUMPT)" ] && [ -x "$(GOIMPORTS)" ] && [ -x "$(GOLANGCI_LINT)" ] && [ -x "$(DEADCODE)" ] && [ "$$(cat $(TOOLS_STAMP) 2>/dev/null)" = "$(TOOLS_VERSION)" ]; then \
		echo "tools up to date"; \
	else \
		set -e; \
		GOBIN=$(TOOLS_DIR) go install mvdan.cc/gofumpt@v0.9.2; \
		GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@v0.44.0; \
		GOBIN=$(TOOLS_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4; \
		GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/deadcode@v0.45.0; \
		printf '%s\n' "$(TOOLS_VERSION)" > "$(TOOLS_STAMP)"; \
	fi

fmt: tools
	@$(GOIMPORTS) -local github.com/steipete/gogcli -w .
	@$(GOFUMPT) -w .

fmt-check: tools
	@set -e; \
	tmp="$$(mktemp)"; \
	trap 'rm -f "$$tmp"' EXIT; \
	$(GOIMPORTS) -local github.com/steipete/gogcli -l . > "$$tmp"; \
	$(GOFUMPT) -l . >> "$$tmp"; \
	unformatted="$$(sort -u "$$tmp")"; \
	if [ -n "$$unformatted" ]; then \
		printf 'formatting needed:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi

lint: tools
	@$(GOLANGCI_LINT) run

deadcode: tools
	@set -e; \
	output_file="$$(mktemp)"; \
	trap 'rm -f "$$output_file"' EXIT; \
	$(DEADCODE) -test ./... > "$$output_file"; \
	if [ "$$(go env GOOS)" != "linux" ]; then \
		GOOS=linux GOARCH=amd64 $(DEADCODE) -test ./... >> "$$output_file"; \
	fi; \
	if [ -s "$$output_file" ]; then \
		cat "$$output_file"; \
		exit 1; \
	fi

pnpm-gate:
	@if [ -f package.json ] || [ -f package.json5 ] || [ -f package.yaml ]; then \
		pnpm lint && pnpm build && pnpm test; \
	else \
		echo "pnpm gate skipped (no package.json)"; \
	fi

test:
	@go test $(GO_TEST_FLAGS) $(TEST_FLAGS) $(TEST_PKGS)
	@node --test scripts/eval-gws.test.mjs scripts/eval-gws-agents.test.mjs

eval-gws: build
	@node scripts/eval-gws.mjs --gog $(BIN) --gws $${GWS_BIN:-gws} --out $${OUT:-/tmp/gog-gws-eval.json}

eval-gws-agents: build
	@node scripts/eval-gws-agents.mjs --gog $(BIN) --gws $${GWS_BIN:-gws} --account "$${GOG_EVAL_ACCOUNT:?set GOG_EVAL_ACCOUNT}" $${GOG_EVAL_DRIVE_NAME:+--drive-name "$${GOG_EVAL_DRIVE_NAME}"} --out $${OUT:-/tmp/gog-gws-agent-eval.json}

eval-gws-test:
	@node --test scripts/eval-gws.test.mjs scripts/eval-gws-agents.test.mjs

ci: pnpm-gate fmt-check lint deadcode test docs-check agent-skills-check

worker-ci:
	@pnpm -C internal/tracking/worker lint
	@pnpm -C internal/tracking/worker build
	@pnpm -C internal/tracking/worker test
