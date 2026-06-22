---
title: Why gog
description: "Why gog combines task-first Google Workspace workflows with stable automation and explicit agent safety."
---

# Why gog

Google Workspace automation usually starts simple: list messages, fetch an
event, update a cell. It becomes operational software as soon as it must choose
an account, preserve machine-readable output, recover from expired auth, avoid
an accidental send, or hand fetched content to an agent.

`gog` is built for that second stage.

## The product idea

Use one command surface everywhere:

```text
person at a terminal → shell script / CI → coding agent / MCP client
                         same gog commands
```

The interface does not fork into a human CLI and an agent API. Commands expose
the same arguments and behavior; global contracts make them safe to compose.

## Five design choices

### 1. Workflows before endpoint spelling

First-class commands encode the parts that raw API calls leave to every user:
calendar-name resolution, pagination, time parsing, Gmail content sanitation,
Drive audit projections, Docs revision controls, dry-run plans, and stable
output shapes.

### 2. Stdout is an API

`--json` and `--plain` keep stdout parseable. Human guidance, warnings,
prompts, and progress use stderr. Stable exit codes let automation branch on
auth, permission, quota, empty-result, not-found, and retryable failures without
matching prose.

### 3. Auth is a routing problem

One installation can hold several Google accounts and several OAuth client
projects. Aliases, domain mappings, direct access tokens, ADC, and Workspace
service accounts all resolve through the same command layer.

### 4. Safety is layered

No single switch covers every caller. gog combines:

- preview/dry-run behavior for mutations;
- confirmation for destructive operations;
- runtime `--readonly` enforcement before API requests are dispatched;
- Gmail no-send policy at flag, config, and account level;
- runtime command allowlists and denylists;
- baked binaries whose blocked commands disappear from help and schema;
- untrusted-content markers for fetched Google text;
- a typed MCP tool surface with writes disabled by default.

### 5. The live CLI generates its contract

`gog schema --json` is the source for the command reference and generated agent
skills. Documentation, agent discovery, and runtime policy inspection therefore
describe the binary that is actually running, including build-time safety
filtering.

## Who it is for

- People who manage more than one Google account from a terminal.
- Teams that need auditable Workspace automation in shell or CI.
- Maintainers who want high-level workflows without hiding the underlying IDs,
  scopes, and API errors.
- Agent builders who need constrained tools and machine-readable contracts.

It is not a hosted integration platform, a replacement for Google Cloud Console
administration, or a promise that every Google API feature has a curated command.
The [command index](commands/) is the exact current surface.

## A useful first proof

After [Quickstart](quickstart.md), run a read-only loop before enabling any
mutation workflow:

```bash
gog auth doctor --check
gog --account you@gmail.com --readonly gmail search 'newer_than:7d' --max 5 --json
gog --account you@gmail.com --readonly calendar events --today --json
gog --account you@gmail.com --readonly drive audit sharing --parent <folderId> --json
gog schema gmail search --json
```

Then choose the narrowest next boundary: keep runtime read-only enforcement,
add an exact command allowlist or no-send policy, use a dedicated OAuth client
or service account, or bake a safety profile into the binary.
