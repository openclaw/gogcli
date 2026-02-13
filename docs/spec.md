# ratatosk spec

## Goal

Build a single, clean, modern Go CLI that talks to:

- Gmail API
- Google Calendar API
- Google Classroom API
- Google Drive API
- Google People API (Contacts + directory)

This replaces the existing separate CLIs (`gmcli`, `gccli`, `gdcli`) and the Python contacts server conceptually, but:

- no backwards compatibility
- no migration tooling

## Non-goals

- Preserving legacy command names/flags/output formats
- Importing existing `~/.gmcli`, `~/.gccli`, `~/.gdcli` state
- Running an MCP server (this is a CLI)

## Language/runtime

- Go `1.25` (see `go.mod`)

## CLI framework

- `github.com/alecthomas/kong`
- Root command: `rata`
- Global flag:
  - `--color=auto|always|never` (default `auto`)
  - `--json` (JSON output to stdout)
  - `--plain` (TSV output to stdout; stable/parseable; disables colors)
  - `--force` (skip confirmations for destructive commands)
  - `--no-input` (never prompt; fail instead)
  - `--version` (print version)

Notes:

- We run `SilenceUsage: true` and print errors ourselves (colored when possible).
- `NO_COLOR` is respected.

Environment:

- `RATA_COLOR=auto|always|never` (default `auto`, overridden by `--color`)
- `RATA_JSON=1` (default JSON output; overridden by flags)
- `RATA_PLAIN=1` (default plain output; overridden by flags)

## Output (TTY-aware colors)

- `github.com/muesli/termenv` is used to detect rich TTY capabilities and render colored output.
- Colors are enabled when:
  - output is a rich terminal and `--color=auto`, and `NO_COLOR` is not set; or
  - `--color=always`
- Colors are disabled when:
  - `--color=never`; or
  - `NO_COLOR` is set

Implementation: `internal/ui/ui.go`.

## Auth + secret storage

### OAuth client credentials (non-secret-ish)

- Stored on disk in the per-user config directory:
  - `$(os.UserConfigDir())/ratatosk/credentials.json` (default client)
  - `$(os.UserConfigDir())/ratatosk/credentials-<client>.json` (named clients)
- Written with mode `0600`.
- Command:
  - `rata auth credentials <credentials.json>`
  - `rata --client <name> auth credentials <credentials.json>`
  - `rata auth credentials list`
- Supports Google’s downloaded JSON format:
  - `installed.client_id/client_secret` or `web.client_id/client_secret`

Implementation: `internal/config/*`.

### Refresh tokens (secrets)

- Stored in OS credential store via `github.com/99designs/keyring`.
- Key namespace is `ratatosk` (keyring `ServiceName`).
- Key format: `token:<client>:<email>` (default client uses `token:default:<email>`)
- Legacy key format: `token:<email>` (migrated on first read)
- Stored payload is JSON (refresh token + metadata like selected services/scopes).
- Fallback: if no OS credential store is available, keyring may use its encrypted "file" backend:
  - Directory: `$(os.UserConfigDir())/ratatosk/keyring/` (one file per key)
  - Password: prompts on TTY; for non-interactive runs set `RATA_KEYRING_PASSWORD`

Current minimal management commands (implemented):

- `rata auth tokens list` (keys only)
- `rata auth tokens delete <email>`

Implementation: `internal/secrets/store.go`.

### OAuth flow

- Desktop OAuth 2.0 flow using local HTTP redirect on an ephemeral port.
- Supports a browserless/manual flow (paste redirect URL) for headless environments.
- Refresh token issuance:
  - requests `access_type=offline`
  - supports `--force-consent` to force the consent prompt when Google doesn't return a refresh token
  - uses `include_granted_scopes=true` to support incremental auth re-runs

Scope selection note:

- The consent screen shows the scopes the CLI requested.
- Users cannot selectively un-check individual requested scopes in the consent screen; they either approve all requested scopes or cancel.
- To request fewer scopes, choose fewer services via `rata auth add --services ...` or use `rata auth add --readonly` where applicable.

## Config layout

- Base config dir: `$(os.UserConfigDir())/ratatosk/`
- Files:
  - `config.json` (JSON5; comments and trailing commas allowed)
  - `credentials.json` (OAuth client id/secret; default client)
  - `credentials-<client>.json` (OAuth client id/secret; named clients)
- State:
  - `state/gmail-watch/<account>.json` (Gmail watch state)
- Secrets:
  - refresh tokens in keyring

We intentionally avoid storing refresh tokens in plain JSON on disk.

Environment:

- `RATA_ACCOUNT=you@gmail.com` (email or alias; used when `--account` is not set; otherwise uses keyring default or a single stored token)
- `RATA_CLIENT=work` (select OAuth client bucket; see `--client`)
- `RATA_KEYRING_PASSWORD=...` (used when keyring falls back to encrypted file backend in non-interactive environments)
- `RATA_KEYRING_BACKEND={auto|keychain|file}` (force backend; use `file` to avoid Keychain prompts and pair with `RATA_KEYRING_PASSWORD` for non-interactive)
- `RATA_TIMEZONE=America/New_York` (default output timezone; IANA name or `UTC`; `local` forces local timezone)
- `RATA_ENABLE_COMMANDS=calendar,tasks` (optional allowlist of top-level commands)
- `config.json` can also set `keyring_backend` (JSON5; env vars take precedence)
- `config.json` can also set `default_timezone` (IANA name or `UTC`)
- `config.json` can also set `account_aliases` for `rata auth alias` (JSON5)
- `config.json` can also set `account_clients` (email -> client) and `client_domains` (domain -> client)

Flag aliases:
- `--out` also accepts `--output`.
- `--out-dir` also accepts `--output-dir` (Gmail thread attachment downloads).

## Commands (current + planned)

### Implemented

- `rata auth credentials <credentials.json|->`
- `rata auth credentials list`
- `rata --client <name> auth credentials <credentials.json|->`
- `rata auth add <email> [--services user|all|gmail,calendar,classroom,drive,docs,contacts,tasks,sheets,people,groups] [--readonly] [--drive-scope full|readonly|file] [--manual] [--force-consent]`
- `rata auth services [--markdown]`
- `rata auth keep <email> --key <service-account.json>` (Google Keep; Workspace only)
- `rata auth list`
- `rata auth alias list`
- `rata auth alias set <alias> <email>`
- `rata auth alias unset <alias>`
- `rata auth status`
- `rata auth remove <email>`
- `rata auth tokens list`
- `rata auth tokens delete <email>`
- `rata config get <key>`
- `rata config keys`
- `rata config list`
- `rata config path`
- `rata config set <key> <value>`
- `rata config unset <key>`
- `rata drive ls [--parent ID] [--max N] [--page TOKEN] [--query Q]`
- `rata drive search <text> [--max N] [--page TOKEN]`
- `rata drive get <fileId>`
- `rata drive download <fileId> [--out PATH]`
- `rata drive upload <localPath> [--name N] [--parent ID] [--convert]`
- `rata drive mkdir <name> [--parent ID]`
- `rata drive delete <fileId>`
- `rata drive move <fileId> --parent ID`
- `rata drive rename <fileId> <newName>`
- `rata drive share <fileId> [--anyone | --email addr] [--role reader|writer] [--discoverable]`
- `rata drive permissions <fileId> [--max N] [--page TOKEN]`
- `rata drive unshare <fileId> <permissionId>`
- `rata drive url <fileIds...>`
- `rata drive drives [--max N] [--page TOKEN] [--query Q]`
- `rata calendar calendars`
- `rata calendar acl <calendarId>`
- `rata calendar events <calendarId> [--from RFC3339] [--to RFC3339] [--max N] [--page TOKEN] [--query Q] [--weekday]`
- `rata calendar event|get <calendarId> <eventId>`
- `RATA_CALENDAR_WEEKDAY=1` defaults `--weekday` for `rata calendar events`
- `rata calendar create <calendarId> --summary S --from DT --to DT [--description D] [--location L] [--attendees a@b.com,c@d.com] [--all-day] [--event-type TYPE]`
- `rata calendar update <calendarId> <eventId> [--summary S] [--from DT] [--to DT] [--description D] [--location L] [--attendees ...] [--add-attendee ...] [--all-day] [--event-type TYPE]`
- `rata calendar delete <calendarId> <eventId>`
- `rata calendar freebusy <calendarIds> --from RFC3339 --to RFC3339`
- `rata calendar respond <calendarId> <eventId> --status accepted|declined|tentative [--send-updates all|none|externalOnly]`
- `rata time now [--timezone TZ]`
- `rata classroom courses [--state ...] [--max N] [--page TOKEN]`
- `rata classroom courses get <courseId>`
- `rata classroom courses create --name NAME [--owner me] [--state ACTIVE|...]`
- `rata classroom courses update <courseId> [--name ...] [--state ...]`
- `rata classroom courses delete <courseId>`
- `rata classroom courses archive <courseId>`
- `rata classroom courses unarchive <courseId>`
- `rata classroom courses join <courseId> [--role student|teacher] [--user me]`
- `rata classroom courses leave <courseId> [--role student|teacher] [--user me]`
- `rata classroom courses url <courseId...>`
- `rata classroom students <courseId> [--max N] [--page TOKEN]`
- `rata classroom students get <courseId> <userId>`
- `rata classroom students add <courseId> <userId> [--enrollment-code CODE]`
- `rata classroom students remove <courseId> <userId>`
- `rata classroom teachers <courseId> [--max N] [--page TOKEN]`
- `rata classroom teachers get <courseId> <userId>`
- `rata classroom teachers add <courseId> <userId>`
- `rata classroom teachers remove <courseId> <userId>`
- `rata classroom roster <courseId> [--students] [--teachers]`
- `rata classroom coursework <courseId> [--state ...] [--topic TOPIC_ID] [--scan-pages N] [--max N] [--page TOKEN]`
- `rata classroom coursework get <courseId> <courseworkId>`
- `rata classroom coursework create <courseId> --title TITLE [--type ASSIGNMENT|...]`
- `rata classroom coursework update <courseId> <courseworkId> [--title ...]`
- `rata classroom coursework delete <courseId> <courseworkId>`
- `rata classroom coursework assignees <courseId> <courseworkId> [--mode ...] [--add-student ...]`
- `rata classroom materials <courseId> [--state ...] [--topic TOPIC_ID] [--scan-pages N] [--max N] [--page TOKEN]`
- `rata classroom materials get <courseId> <materialId>`
- `rata classroom materials create <courseId> --title TITLE`
- `rata classroom materials update <courseId> <materialId> [--title ...]`
- `rata classroom materials delete <courseId> <materialId>`
- `rata classroom submissions <courseId> <courseworkId> [--state ...] [--max N] [--page TOKEN]`
- `rata classroom submissions get <courseId> <courseworkId> <submissionId>`
- `rata classroom submissions turn-in <courseId> <courseworkId> <submissionId>`
- `rata classroom submissions reclaim <courseId> <courseworkId> <submissionId>`
- `rata classroom submissions return <courseId> <courseworkId> <submissionId>`
- `rata classroom submissions grade <courseId> <courseworkId> <submissionId> [--draft N] [--assigned N]`
- `rata classroom announcements <courseId> [--state ...] [--max N] [--page TOKEN]`
- `rata classroom announcements get <courseId> <announcementId>`
- `rata classroom announcements create <courseId> --text TEXT`
- `rata classroom announcements update <courseId> <announcementId> [--text ...]`
- `rata classroom announcements delete <courseId> <announcementId>`
- `rata classroom announcements assignees <courseId> <announcementId> [--mode ...]`
- `rata classroom topics <courseId> [--max N] [--page TOKEN]`
- `rata classroom topics get <courseId> <topicId>`
- `rata classroom topics create <courseId> --name NAME`
- `rata classroom topics update <courseId> <topicId> --name NAME`
- `rata classroom topics delete <courseId> <topicId>`
- `rata classroom invitations [--course ID] [--user ID]`
- `rata classroom invitations get <invitationId>`
- `rata classroom invitations create <courseId> <userId> --role STUDENT|TEACHER|OWNER`
- `rata classroom invitations accept <invitationId>`
- `rata classroom invitations delete <invitationId>`
- `rata classroom guardians <studentId> [--max N] [--page TOKEN]`
- `rata classroom guardians get <studentId> <guardianId>`
- `rata classroom guardians delete <studentId> <guardianId>`
- `rata classroom guardian-invitations <studentId> [--state ...] [--max N] [--page TOKEN]`
- `rata classroom guardian-invitations get <studentId> <invitationId>`
- `rata classroom guardian-invitations create <studentId> --email EMAIL`
- `rata classroom profile [userId]`
- `rata gmail search <query> [--max N] [--page TOKEN]`
- `rata gmail messages search <query> [--max N] [--page TOKEN] [--include-body]`
- `rata gmail thread get <threadId> [--download]`
- `rata gmail thread modify <threadId> [--add ...] [--remove ...]`
- `rata gmail get <messageId> [--format full|metadata|raw] [--headers ...]`
- `rata gmail attachment <messageId> <attachmentId> [--out PATH] [--name NAME]`
- `rata gmail url <threadIds...>`
- `rata gmail labels list`
- `rata gmail labels get <labelIdOrName>`
- `rata gmail labels create <name>`
- `rata gmail labels modify <threadIds...> [--add ...] [--remove ...]`
- `rata gmail send --to a@b.com --subject S [--body B] [--body-html H] [--cc ...] [--bcc ...] [--reply-to-message-id <messageId>] [--reply-to addr] [--attach <file>...]`
- `rata gmail drafts list [--max N] [--page TOKEN]`
- `rata gmail drafts get <draftId> [--download]`
- `rata gmail drafts create --subject S [--to a@b.com] [--body B] [--body-html H] [--cc ...] [--bcc ...] [--reply-to-message-id <messageId>] [--reply-to addr] [--attach <file>...]`
- `rata gmail drafts update <draftId> --subject S [--to a@b.com] [--body B] [--body-html H] [--cc ...] [--bcc ...] [--reply-to-message-id <messageId>] [--reply-to addr] [--attach <file>...]`
- `rata gmail drafts send <draftId>`
- `rata gmail drafts delete <draftId>`
- `rata gmail watch start|status|renew|stop|serve`
- `rata gmail history --since <historyId>`
- `rata chat spaces list [--max N] [--page TOKEN]`
- `rata chat spaces find <displayName> [--max N]`
- `rata chat spaces create <displayName> [--member email,...]`
- `rata chat messages list <space> [--max N] [--page TOKEN] [--order ORDER] [--thread THREAD] [--unread]`
- `rata chat messages send <space> --text TEXT [--thread THREAD]`
- `rata chat threads list <space> [--max N] [--page TOKEN]`
- `rata chat dm space <email>`
- `rata chat dm send <email> --text TEXT [--thread THREAD]`
- `rata tasks lists [--max N] [--page TOKEN]`
- `rata tasks lists create <title>`
- `rata tasks list <tasklistId> [--max N] [--page TOKEN]`
- `rata tasks get <tasklistId> <taskId>`
- `rata tasks add <tasklistId> --title T [--notes N] [--due RFC3339|YYYY-MM-DD] [--repeat daily|weekly|monthly|yearly] [--repeat-count N] [--repeat-until DT] [--parent ID] [--previous ID]`
- `rata tasks update <tasklistId> <taskId> [--title T] [--notes N] [--due RFC3339|YYYY-MM-DD] [--status needsAction|completed]`
- `rata tasks done <tasklistId> <taskId>`
- `rata tasks undo <tasklistId> <taskId>`
- `rata tasks delete <tasklistId> <taskId>`
- `rata tasks clear <tasklistId>`
- `rata contacts search <query> [--max N]`
- `rata contacts list [--max N] [--page TOKEN]`
- `rata contacts get <people/...|email>`
- `rata contacts create --given NAME [--family NAME] [--email addr] [--phone num]`
- `rata contacts update <people/...> [--given NAME] [--family NAME] [--email addr] [--phone num]`
- `rata contacts delete <people/...>`
- `rata contacts directory list [--max N] [--page TOKEN]`
- `rata contacts directory search <query> [--max N] [--page TOKEN]`
- `rata contacts other list [--max N] [--page TOKEN]`
- `rata contacts other search <query> [--max N]`
- `rata people me`
- `rata people get <people/...|userId>`
- `rata people search <query> [--max N] [--page TOKEN]`
- `rata people relations [<people/...|userId>] [--type TYPE]`

### Planned high-level command tree

- `rata auth …`
  - `rata auth credentials <credentials.json>`
  - `rata auth credentials list`
  - `rata --client <name> auth credentials <credentials.json>`
- `rata gmail …`
- `rata chat …`
- `rata calendar …`
- `rata drive …`
- `rata contacts …`
- `rata tasks …`
- `rata people …`

Planned service identifiers (canonical):

- `gmail`
- `calendar`
- `chat`
- `drive`
- `contacts`
- `tasks`
- `people`

## Google API dependencies (planned)

- `golang.org/x/oauth2`
- `golang.org/x/oauth2/google`
- `google.golang.org/api/option`
- `google.golang.org/api/gmail/v1`
- `google.golang.org/api/calendar/v3`
- `google.golang.org/api/chat/v1`
- `google.golang.org/api/drive/v3`
- `google.golang.org/api/people/v1`
- `google.golang.org/api/tasks/v1`

## Scopes (planned)

We store a single refresh token per Google account email.

- `rata auth add` requests a union of scopes based on `--services`.
- Each API client refreshes an access token for the subset of scopes needed for that service.
- If you later want additional services, re-run `rata auth add <email> --services ...` (may require `--force-consent` to mint a new refresh token).

- Gmail: `https://mail.google.com/` (or narrower scopes if we decide later)
- Calendar: `https://www.googleapis.com/auth/calendar`
- Chat:
  - `https://www.googleapis.com/auth/chat.spaces`
  - `https://www.googleapis.com/auth/chat.messages`
  - `https://www.googleapis.com/auth/chat.memberships`
  - `https://www.googleapis.com/auth/chat.users.readstate.readonly`
- Drive: `https://www.googleapis.com/auth/drive`
- Contacts/Directory:
  - `https://www.googleapis.com/auth/contacts`
  - `https://www.googleapis.com/auth/contacts.other.readonly`
  - `https://www.googleapis.com/auth/directory.readonly`
- People:
  - `profile` (OIDC)

## Output formats

Default: human-friendly tables (stdlib `text/tabwriter`).

- Parseable stdout:
  - `--json`: JSON objects/arrays suitable for scripting
  - `--plain`: stable TSV (tabs preserved; no alignment; no colors)
- Human-facing hints/progress are written to stderr so stdout can be safely captured.
- Colors are only used for human-facing output and are disabled automatically for `--json` and `--plain`.

We avoid heavy table deps unless we decide we need them.

## Code layout (current)

- `cmd/rata/main.go` — binary entrypoint
- `internal/cmd/*` — kong command structs
- `internal/ui/*` — color + printing
- `internal/config/*` — config paths + credential parsing/writing
- `internal/secrets/*` — keyring store

## Formatting, linting, tests

### Formatting

Pinned tools, installed into local `.tools/` via `make tools`:

- `mvdan.cc/gofumpt@v0.7.0`
- `golang.org/x/tools/cmd/goimports@v0.38.0`
- `github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.2`

Commands:

- `make fmt` — applies `goimports` + `gofumpt`
- `make fmt-check` — formats and fails if Go files or `go.mod/go.sum` change

### Lint

- `golangci-lint` with config in `.golangci.yml`
- `make lint`

### Tests

- stdlib `testing` (+ `httptest` when we add OAuth/API tests)
- `make test`

### Integration tests (local only)

There is an opt-in integration test suite guarded by build tags (not run in CI).

- Requires:
  - stored `credentials.json` (or `credentials-<client>.json`) via `rata auth credentials ...`
  - refresh token in keyring via `rata auth add <email>`
- Run:
  - `RATA_IT_ACCOUNT=you@gmail.com go test -tags=integration ./internal/integration`
  - optional: `RATA_CLIENT=work` to select a non-default OAuth client

## CI (GitHub Actions)

Workflow: `.github/workflows/ci.yml`

- runs on push + PR
- uses `actions/setup-go` with `go-version-file: go.mod`
- runs:
  - `make tools`
  - `make fmt-check`
  - `go test ./...`
  - `golangci-lint` (pinned `v1.62.2`)

## Next implementation steps

- Expand Gmail further (labels by name everywhere, richer body rendering, compose edge cases).
- Improve People updates (multi-field + richer contact data).
- Harden UX (consistent output formats, retries/backoff on specific transient errors).
