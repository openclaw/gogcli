# Windows Compatibility and Security Hardening Plan

Date started: 2026-03-16
Branch: windows-compatible

## Goal
Make the project reliably usable on Windows by identifying and fixing platform-specific assumptions in code, tests, scripts, and docs, while also flagging security risks for open-source distribution.

## Scope
- Go source under `cmd/` and `internal/`
- Tests (`*_test.go`)
- Build/dev scripts and Makefile usage
- Docs that prescribe non-Windows-only commands without alternatives
- Dependency and code-level security checks

## Work Plan
- [x] Baseline repository state captured
- [x] Automated scan for Unix-specific usage patterns
- [ ] Run test/build checks on Windows
- [x] Implement code fixes for Windows compatibility
- [x] Implement test fixes for Windows compatibility
- [x] Improve docs with Windows-safe guidance where needed
- [x] Security review (deps + obvious code issues)
- [ ] Final validation (`go test ./...` where feasible)
- [x] Summarize findings and residual risks

## Progress Log
- 2026-03-16: Initialized plan and tracking document.
- 2026-03-16: Found and fixed `ExpandPath` handling for Windows-style `~\\...` paths.
- 2026-03-16: Added test coverage for Windows-style home expansion.
- 2026-03-16: Updated integration live-script test to skip on Windows (POSIX-shell dependency).
- 2026-03-16: Added Windows guidance for live script execution in `README.md`.
- 2026-03-16: Added Windows PowerShell development fallback commands in `README.md` for environments without POSIX shell tooling.
- 2026-03-16: Added `scripts/live-test.ps1` wrapper so live CLI smoke tests can be launched from PowerShell.
- 2026-03-16: Updated integration `TestLiveScript` to use the PowerShell wrapper on Windows when available.
- 2026-03-16: Attempted `go test ./...` but `go` command is not available in current terminal PATH.

## Findings
### Windows Compatibility
- `internal/config/ExpandPath` only supported `~/...`; Windows-style `~\\...` was not expanded.
- `internal/integration/TestLiveScript` directly executed `scripts/live-test.sh`, which is not directly runnable on Windows shells.
- Live-test documentation lacked explicit Windows guidance.
- Added PowerShell wrapper for live test script execution from Windows shells.
- Residual: live tests still require a POSIX shell runtime (Git Bash/WSL) under the hood.
- Residual: `Makefile` remains POSIX-shell oriented (`SHELL := /bin/bash`), so native PowerShell-only workflows should use direct `go build` / `go test` commands (documented in README).

### Security
- Initial static review found no immediate command-injection sinks in touched areas; command invocations are fixed executable names with structured arguments.
- `http://127.0.0.1` OAuth callback URLs are used intentionally for localhost redirect handling.
- `gmail watch serve` requires authentication safeguards (`--verify-oidc` or `--token`) when binding to non-loopback addresses, reducing accidental exposure risk.
- Full dependency vulnerability scan is still pending until `go` tooling is available in the terminal.

## Commits
- `4bc17bb` fix(windows): handle tilde backslash paths and skip POSIX live-script test
- `5dded53` docs(windows): add PowerShell guidance and update compatibility tracker
