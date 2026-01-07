# Changelog

## 0.5.0 - Unreleased

- Config: add JSON5 `config.json` (comments ok) and `gog auth status`/help now show keyring backend + config path.
- CLI: help now defaults to grouped output (no expanded subcommands); use `GOG_HELP=full gog --help` for full expansion.
- CLI: help shows build version + git SHA and adds colored headings/command names (respects `NO_COLOR` and `--color`).
- Auth: `gog auth list --check` validates refresh tokens by exchanging for an access token.
- Auth: OAuth browser flow now finishes immediately after callback (no 30s “stuck” delay).
- Homebrew: tap now installs GitHub release binaries (macOS) to reduce Keychain prompt churn.
- Secrets: add `GOG_KEYRING_BACKEND={auto|keychain|file}` to force backend (use `file` to avoid Keychain prompts; pair with `GOG_KEYRING_PASSWORD`).
- Secrets: normalize keyring backend values (trim/case-insensitive) + add coverage (#26) — thanks @salmonumbrella.
- Sheets: `sheets update|append --copy-validation-from ...` copies data validation onto written cells (#29) — thanks @mahmoudashraf93.
- Docs: explain macOS Keychain prompts and backend options.
- DX: remove pnpm wrapper; use `make gog`.
- DX: `make gogcli -- ...` passes args to the CLI; add `make gogcli-help` convenience target.
- Calendar: `gog calendar update --add-attendee ...` adds attendees without replacing existing RSVP state (#24) — thanks @salmonumbrella.
- Gmail: `gog gmail thread attachments` list/download attachments (#27) — thanks @salmonumbrella.
- Gmail: `gog gmail thread get --full` shows complete bodies (default truncates) (#25) — thanks @salmonumbrella.
- Gmail: reorganize `gog gmail --help` into sections and add `gog gmail settings ...` (old subcommands remain available).

## 0.4.2 - 2025-12-31

- Gmail: `thread modify` subcommand + `thread get` split (#21) — thanks @alexknowshtml.
- Auth: refreshed account manager + success UI (#20) — thanks @salmonumbrella.
- CLI: migrate from Cobra to Kong (same commands/flags; help/validation wording may differ slightly).
- DX: tighten golangci-lint rules and fix new findings.
- Security: config/attachment/export dirs now created with 0700 permissions.

## 0.4.1 - 2025-12-28

- macOS: release binaries now built with cgo so Keychain backend works (no encrypted file-store fallback / password prompts; Issue #19).

## 0.4.0 - 2025-12-26

### Added

- Resilience: automatic retries + circuit breaker for Google API calls (429/5xx).
- Gmail: batch ops + settings commands (autoforward, delegates, filters, forwarding, send-as, vacation).
- Gmail: `gog gmail thread --download --out-dir ...` for saving thread attachments to a specific directory.
- Calendar: colors, conflicts, search, multi-timezone time.
- Sheets: read/write/update/append/clear + create spreadsheets.
- Sheets: copy spreadsheets via Drive (`gog sheets copy ...`).
- Drive: `gog drive download --format ...` for Google Docs exports (e.g. Sheets to PDF/XLSX, Docs to PDF/DOCX/TXT, Slides to PDF/PPTX).
- Drive: copy files (`gog drive copy ...`).
- Docs/Slides/Sheets: dedicated export commands (`gog docs export`, `gog slides export`, `gog sheets export`).
- Docs: create/copy (`gog docs create`, `gog docs copy`) and print plain text (`gog docs cat`).
- Slides: create/copy (`gog slides create`, `gog slides copy`).
- Auth: browser-based accounts manager (`gog auth manage`).
- DX: shell completion (`gog completion ...`) and `--verbose` logging.

### Fixed

- Gmail: `gog gmail attachment` download now works reliably; avoid re-fetching payload for filename inference and accept padded base64 responses.
- Gmail: `gog gmail thread --download` now saves attachments to the current directory by default and creates missing output directories.
- Sheets: avoid flag collision with global `--json`; values input flag is now `--values-json` for `sheets update|append`.

### Changed

- Internal: reduce duplicate code for Drive-backed exports and tabular/paging output; embed auth UI templates as HTML assets.

## 0.3.0 - 2025-12-26

### Added

- Calendar: `gog calendar calendars` and `gog calendar acl` now support `--max` and `--page` (JSON includes `nextPageToken`).
- Drive: `gog drive permissions` now supports `--max` and `--page` (JSON includes `nextPageToken`).

### Changed

- macOS: stop trying to modify Keychain ACLs (“trust gog”); removed `GOG_KEYCHAIN_TRUST_APPLICATION`.
- BREAKING: remove positional/legacy flags; normalize paging and file output flags.
- BREAKING: replace `--output` with `--json` and `--plain` (and env `GOG_OUTPUT` with `GOG_JSON`/`GOG_PLAIN`).
- BREAKING: destructive commands now require `--force` in non-interactive contexts (or they prompt on TTY).
- BREAKING: `gog calendar create|update` uses `--from/--to` (removed `--start/--end`).
- BREAKING: `gog gmail send|drafts create` uses `--reply-to-message-id` (removed `--reply-to` for message IDs) and `--reply-to` (removed `--reply-to-address`).
- BREAKING: `gog gmail attachment` uses `--name` (removed `--filename`).
- BREAKING: Drive: `drive ls` uses `--parent` (removed positional `folderId`), `drive upload` uses `--parent` (removed `--folder`), `drive move` uses `--parent` (removed positional `newParentId`).
- BREAKING: `gog drive download` uses `--out` (removed positional `destPath`).
- BREAKING: `gog auth tokens export` uses `--out` (removed positional `outPath`).
- BREAKING: `gog auth tokens export` uses `--overwrite` (removed `--force`).

## 0.2.1 - 2025-12-26

### Fixed

- macOS: reduce repeated Keychain password prompts by trusting the `gog` binary by default (set `GOG_KEYCHAIN_TRUST_APPLICATION=0` to disable).

## 0.2.0 - 2025-12-24

### Added

- Gmail: watch + Pub/Sub push handler (`gog gmail watch start|status|renew|stop|serve`) with optional webhook forwarding, include-body, and max-bytes.
- Gmail: history listing via `gog gmail history --since <historyId>`.
- Gmail: HTML bodies for `gmail send` and `gmail drafts create` via `--body-html` (multipart/alternative when combined with `--body`, PR #16 — thanks @shanelindsay).
- Gmail: `--reply-to-address` (sets `Reply-To` header, PR #16 — thanks @shanelindsay).
- Tasks: manage tasklists and tasks (`lists`, `list`, `add`, `update`, `done`, `undo`, `delete`, `clear`, PR #10 — thanks @shanelindsay).
### Changed

- Build: `make` builds `./bin/gog` by default (adds `build` target, PR #12 — thanks @advait).
- Docs: local build instructions now use `make` (PR #12 — thanks @advait).

### Fixed

- Secrets: keyring file-backend fallback now stores encrypted entries in `$(os.UserConfigDir())/gogcli/keyring/` and supports non-interactive via `GOG_KEYRING_PASSWORD` (PR #13 — thanks @advait).
- Gmail: decode base64url attachment/message-part payloads (PR #15 — thanks @shanelindsay).
- Auth: add `people` service (OIDC `profile` scope) so `gog people me` works with `gog auth add --services all`.

## 0.1.1 - 2025-12-17

### Added

- Calendar: respond to invites via `gog calendar respond <calendarId> <eventId> --status accepted|declined|tentative` (optional `--send-updates`).
- People: `gog people me` (quick “me card” / `people/me`).
- Gmail: message get via `gog gmail get <messageId> [--format full|metadata|raw]`.
- Gmail: download a single attachment via `gog gmail attachment <messageId> <attachmentId> [--out PATH]`.

## 0.1.0 - 2025-12-12

Initial public release of `gog`: a single Go CLI that unifies Gmail, Calendar, Drive, and Contacts (People API).

### Added

- Unified CLI (`gog`) with service subcommands: `gmail`, `calendar`, `drive`, `contacts`, plus `auth`.
- OAuth setup and account management:
  - Store OAuth client credentials: `gog auth credentials <credentials.json>`.
  - Authorize accounts and store refresh tokens securely via OS keychain using `github.com/99designs/keyring`.
  - List/remove accounts: `gog auth list`, `gog auth remove <email>`.
  - Token management helpers: `gog auth tokens list|delete|export|import`.
- Consistently parseable output:
  - `--output=text` (tab-separated lists on stdout) and `--output=json` (JSON on stdout).
  - Human hints/progress/errors go to stderr.
- Colorized output in rich TTY (`--color=auto|always|never`), automatically disabled for JSON output.
- Gmail features:
  - Search threads, show thread, generate web URLs.
  - Label listing/get (including counts) and thread label modify.
  - Send mail (supports reply headers + attachments).
  - Drafts: list/get/create/send/delete.
- Calendar features:
  - List calendars, list ACL rules.
  - List/get/create/update/delete events and free/busy queries.
- Drive features:
  - List/search/get files, download (including Google Docs export), upload, mkdir, delete, move, rename.
  - Sharing helpers: share/unshare/permissions, and web URL output.
- Contacts / People API features:
  - Contacts list/search/get/create/update/delete.
  - “Other contacts” list/search.
  - Workspace directory list/search (Workspace accounts only).
- Developer experience:
  - Formatting via `gofumpt` + `goimports` (and `gofmt` implicitly) using `make fmt` / `make fmt-check`.
  - Linting via pinned `golangci-lint` with repo config.
  - Tests using stdlib `testing` + `httptest`, with steadily increased unit coverage.
  - GitHub Actions CI running format checks, tests, and lint.
  - `make` builds `./bin/gog` for local dev (`make && ./bin/gog auth add you@gmail.com`).

### Notes / Known Limitations

- Importing tokens into macOS Keychain may require a local (GUI) session; headless/SSH sessions can fail due to Keychain user-interaction restrictions.
- Workspace directory commands require a Google Workspace account; `@gmail.com` accounts will not work for directory endpoints.
