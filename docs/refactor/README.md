---
summary: "Refactor notes (implementation status + next wins)"
read_when:
  - Touching exports/output/templates
  - Planning cleanup work
---

# Refactor notes

Shipped (today)

- `exports.md`: Drive-backed export command pattern (`docs|slides|sheets`).
- `output.md`: shared table + paging helpers.
- `templates.md`: googleauth HTML templates via `//go:embed`.
- Windows compatibility pass:
  - `config.ExpandPath` handles both `~/...` and `~\\...`.
  - integration live tests support Windows via `scripts/live-test.ps1` wrapper.
  - README now documents Windows-native build/auth/live-test flows.
  - full `go test ./...` and `govulncheck` validation completed on Windows.

Backlog / next wins

- `options.md`: ideas; pick + execute when touching adjacent code.

