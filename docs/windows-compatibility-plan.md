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
- [ ] Security review (deps + obvious code issues)
- [ ] Final validation (`go test ./...` where feasible)
- [ ] Summarize findings and residual risks

## Progress Log
- 2026-03-16: Initialized plan and tracking document.
- 2026-03-16: Found and fixed `ExpandPath` handling for Windows-style `~\\...` paths.
- 2026-03-16: Added test coverage for Windows-style home expansion.
- 2026-03-16: Updated integration live-script test to skip on Windows (POSIX-shell dependency).
- 2026-03-16: Added Windows guidance for live script execution in `README.md`.
- 2026-03-16: Attempted `go test ./...` but `go` command is not available in current terminal PATH.

## Findings
### Windows Compatibility
- `internal/config/ExpandPath` only supported `~/...`; Windows-style `~\\...` was not expanded.
- `internal/integration/TestLiveScript` directly executed `scripts/live-test.sh`, which is not directly runnable on Windows shells.
- Live-test documentation lacked explicit Windows guidance.

### Security
- Initial static review found no immediate command-injection sinks in touched areas; command invocations are fixed executable names with structured arguments.
- Full dependency vulnerability scan is pending until `go` tooling is available in the terminal.

## Commits
- Pending (next step in progress)
