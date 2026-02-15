---
summary: "Output helpers (tables + paging)"
read_when:
  - Adding/changing list commands
  - Touching pagination output
---

# Output helpers (tables + paging)

Goal: kill copy/paste; keep output consistent.

## Tables

Use `internal/cmd/output_helpers.go:tableWriter(ctx)`:

- `--plain`: `os.Stdout` (no alignment, TSV-friendly)
- default: `tabwriter.Writer` (aligned columns)

Call pattern:

- `w, flush := tableWriter(cmd.Context())`
- `defer flush()`

## Pagination hint

Use `internal/cmd/output_helpers.go:printNextPageHint(u, token)`:

- prints to stderr
- exact format (tests depend on it): `# Next page: --page <token>`

## Result (Key-Value) Output

Use `internal/cmd/output_helpers.go:writeResult(ctx, u, ...)` for simple command results (especially destructive operations).

Rules:

- Same keys in `--plain` and `--json` (no snake_case vs camelCase split).
- Prefer explicit booleans for flags (e.g. `deleted`, `removed`, `trashed`).
- Include primary identifiers (`id`, `fileId`, etc) as separate keys.
