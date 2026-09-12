---
summary: "Shared command helpers and ownership boundaries"
read_when:
  - Planning cleanup work
  - Touching retry/logging/output plumbing
---

# Shared command helpers

Before adding a helper, check the existing owner:

- `internal/cmd/service_helpers.go` selects accounts and initializes services.
- `internal/cmd/paging.go` collects bounded or unbounded result pages;
  `paging_guard.go` rejects repeated tokens for command-specific loops.
- `internal/cmd/output_helpers.go` routes table and result output through the
  invocation context and prints pagination hints on stderr.
- `internal/cmd/drive_download.go` resolves Drive export formats;
  `export_via_drive.go` orchestrates typed document exports.
- `internal/googleapi/transport.go` handles HTTP retries and replayable bodies.
  `WithoutRetries` is available for operations whose callers require a single
  attempt; do not add a second generic retry loop around it.

Keep service-specific pagination limits, partial-result rules, and output
shapes at the command boundary. Share mechanics without silently changing
those contracts. Existing fake-server tests next to the affected commands
provide request and response examples.
