# gogcli spec

## Goal

Build a single, clean, modern Go CLI that talks to:

- Gmail API
- Google Calendar API
- Google Chat API
- Google Classroom API
- Google Drive API
- Google Docs API
- Google Sheets API
- Google Forms API
- Apps Script API
- Google Tasks API
- Cloud Identity API (Groups)
- Google People API (Contacts + directory)
- Google Keep API (Workspace-only, service account)

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
- Root command: `gog`
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

- `GOG_COLOR=auto|always|never` (default `auto`, overridden by `--color`)
- `GOG_JSON=1` (default JSON output; overridden by flags)
- `GOG_PLAIN=1` (default plain output; overridden by flags)

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
  - `$(os.UserConfigDir())/gogcli/credentials.json` (default client)
  - `$(os.UserConfigDir())/gogcli/credentials-<client>.json` (named clients)
- Written with mode `0600`.
- Command:
  - `gog auth credentials <credentials.json>`
  - `gog --client <name> auth credentials <credentials.json>`
  - `gog auth credentials list`
- Supports Google’s downloaded JSON format:
  - `installed.client_id/client_secret` or `web.client_id/client_secret`

Implementation: `internal/config/*`.

### Refresh tokens (secrets)

- Stored in OS credential store via `github.com/99designs/keyring`.
- Key namespace is `gogcli` (keyring `ServiceName`).
- Key format: `token:<client>:<email>` (default client uses `token:default:<email>`)
- Legacy key format: `token:<email>` (migrated on first read)
- Stored payload is JSON (refresh token + metadata like selected services/scopes).
- Fallback: if no OS credential store is available, keyring may use its encrypted "file" backend:
  - Directory: `$(os.UserConfigDir())/gogcli/keyring/` (one file per key)
  - Password: prompts on TTY; for non-interactive runs set `GOG_KEYRING_PASSWORD`

Current minimal management commands (implemented):

- `gog auth tokens list` (keys only)
- `gog auth tokens delete <email>`

Implementation: `internal/secrets/store.go`.

### OAuth flow

- Desktop OAuth 2.0 flow using local HTTP redirect on an ephemeral port.
- Supports a browserless/manual flow (paste redirect URL) for headless environments.
- Supports a remote/server-friendly 2-step manual flow:
  - Step 1 prints an auth URL (`gog auth add ... --remote --step 1`)
  - Step 2 exchanges the pasted redirect URL and requires `state` validation (`--remote --step 2 --auth-url ...`)
- Refresh token issuance:
  - requests `access_type=offline`
  - supports `--force-consent` to force the consent prompt when Google doesn't return a refresh token
  - uses `include_granted_scopes=true` to support incremental auth re-runs

Scope selection note:

- The consent screen shows the scopes the CLI requested.
- Users cannot selectively un-check individual requested scopes in the consent screen; they either approve all requested scopes or cancel.
- To request fewer scopes, choose fewer services via `gog auth add --services ...` or use `gog auth add --readonly` where applicable.

## Config layout

- Base config dir: `$(os.UserConfigDir())/gogcli/`
- Files:
  - `config.json` (JSON5; comments and trailing commas allowed)
  - `credentials.json` (OAuth client id/secret; default client)
  - `credentials-<client>.json` (OAuth client id/secret; named clients)
- State:
  - `state/gmail-watch/<account>.json` (Gmail watch state)
  - `oauth-manual-state-<state>.json` (temporary manual OAuth state cache; expires quickly; no tokens)
- Secrets:
  - refresh tokens in keyring

We intentionally avoid storing refresh tokens in plain JSON on disk.

Environment:

- `GOG_ACCOUNT=you@gmail.com` (email or alias; used when `--account` is not set; otherwise uses keyring default or a single stored token)
- `GOG_CLIENT=work` (select OAuth client bucket; see `--client`)
- `GOG_KEYRING_PASSWORD=...` (used when keyring falls back to encrypted file backend in non-interactive environments)
- `GOG_KEYRING_BACKEND={auto|keychain|file}` (force backend; use `file` to avoid Keychain prompts and pair with `GOG_KEYRING_PASSWORD` for non-interactive)
- `GOG_TIMEZONE=America/New_York` (default output timezone; IANA name or `UTC`; `local` forces local timezone)
- `GOG_ENABLE_COMMANDS=calendar,tasks` (optional allowlist of top-level commands)
- `config.json` can also set `keyring_backend` (JSON5; env vars take precedence)
- `config.json` can also set `default_timezone` (IANA name or `UTC`)
- `config.json` can also set `account_aliases` for `gog auth alias` (JSON5)
- `config.json` can also set `account_clients` (email -> client) and `client_domains` (domain -> client)

Flag aliases:
- `--out` also accepts `--output`.
- `--out-dir` also accepts `--output-dir` (Gmail thread attachment downloads).

## Commands (current + planned)

### Implemented

- `gog auth credentials <credentials.json|->`
- `gog auth credentials list`
- `gog --client <name> auth credentials <credentials.json|->`
- `gog auth add <email> [--services user|all|gmail,calendar,classroom,drive,docs,contacts,tasks,sheets,people,groups] [--readonly] [--drive-scope full|readonly|file] [--manual] [--remote] [--step 1|2] [--auth-url URL] [--timeout DURATION] [--force-consent]`
- `gog auth services [--markdown]`
- `gog auth keep <email> --key <service-account.json>` (Google Keep; Workspace only)
- `gog auth list`
- `gog auth alias list`
- `gog auth alias set <alias> <email>`
- `gog auth alias unset <alias>`
- `gog auth status`
- `gog auth remove <email>`
- `gog auth tokens list`
- `gog auth tokens delete <email>`
- `gog config get <key>`
- `gog config keys`
- `gog config list`
- `gog config path`
- `gog config set <key> <value>`
- `gog config unset <key>`
- `gog version`
- `gog drive ls [--parent ID] [--max N] [--page TOKEN] [--query Q] [--[no-]all-drives]`
- `gog drive search <text> [--raw-query] [--max N] [--page TOKEN] [--[no-]all-drives]`
- `gog drive get <fileId>`
- `gog drive download <fileId> [--out PATH] [--format F]` (`--format` only applies to Google Workspace files)
- `gog drive upload <localPath> [--name N] [--parent ID] [--convert] [--convert-to doc|sheet|slides]`
- `gog drive mkdir <name> [--parent ID]`
- `gog drive delete <fileId> [--permanent]`
- `gog drive move <fileId> --parent ID`
- `gog drive rename <fileId> <newName>`
- `gog drive share <fileId> --to anyone|user|domain [--email addr] [--domain example.com] [--role reader|writer] [--discoverable]`
- `gog drive permissions <fileId> [--max N] [--page TOKEN]`
- `gog drive unshare <fileId> <permissionId>`
- `gog drive url <fileIds...>`
- `gog drive drives [--max N] [--page TOKEN] [--query Q]`
- `gog calendar calendars`
- `gog calendar acl <calendarId>`
- `gog calendar events <calendarId> [--from RFC3339] [--to RFC3339] [--max N] [--page TOKEN] [--query Q] [--weekday]`
- `gog calendar event|get <calendarId> <eventId>`
- `GOG_CALENDAR_WEEKDAY=1` defaults `--weekday` for `gog calendar events`
- `gog calendar create <calendarId> --summary S --from DT --to DT [--description D] [--location L] [--attendees a@b.com,c@d.com] [--all-day] [--event-type TYPE]`
- `gog calendar update <calendarId> <eventId> [--summary S] [--from DT] [--to DT] [--description D] [--location L] [--attendees ...] [--add-attendee ...] [--all-day] [--event-type TYPE]`
- `gog calendar delete <calendarId> <eventId>`
- `gog calendar freebusy <calendarIds> --from RFC3339 --to RFC3339`
- `gog calendar respond <calendarId> <eventId> --status accepted|declined|tentative [--send-updates all|none|externalOnly]`
- `gog time now [--timezone TZ]`
- `gog classroom courses [--state ...] [--max N] [--page TOKEN]`
- `gog classroom courses get <courseId>`
- `gog classroom courses create --name NAME [--owner me] [--state ACTIVE|...]`
- `gog classroom courses update <courseId> [--name ...] [--state ...]`
- `gog classroom courses delete <courseId>`
- `gog classroom courses archive <courseId>`
- `gog classroom courses unarchive <courseId>`
- `gog classroom courses join <courseId> [--role student|teacher] [--user me]`
- `gog classroom courses leave <courseId> [--role student|teacher] [--user me]`
- `gog classroom courses url <courseId...>`
- `gog classroom students <courseId> [--max N] [--page TOKEN]`
- `gog classroom students get <courseId> <userId>`
- `gog classroom students add <courseId> <userId> [--enrollment-code CODE]`
- `gog classroom students remove <courseId> <userId>`
- `gog classroom teachers <courseId> [--max N] [--page TOKEN]`
- `gog classroom teachers get <courseId> <userId>`
- `gog classroom teachers add <courseId> <userId>`
- `gog classroom teachers remove <courseId> <userId>`
- `gog classroom roster <courseId> [--students] [--teachers]`
- `gog classroom coursework <courseId> [--state ...] [--topic TOPIC_ID] [--scan-pages N] [--max N] [--page TOKEN]`
- `gog classroom coursework get <courseId> <courseworkId>`
- `gog classroom coursework create <courseId> --title TITLE [--type ASSIGNMENT|...]`
- `gog classroom coursework update <courseId> <courseworkId> [--title ...]`
- `gog classroom coursework delete <courseId> <courseworkId>`
- `gog classroom coursework assignees <courseId> <courseworkId> [--mode ...] [--add-student ...]`
- `gog classroom materials <courseId> [--state ...] [--topic TOPIC_ID] [--scan-pages N] [--max N] [--page TOKEN]`
- `gog classroom materials get <courseId> <materialId>`
- `gog classroom materials create <courseId> --title TITLE`
- `gog classroom materials update <courseId> <materialId> [--title ...]`
- `gog classroom materials delete <courseId> <materialId>`
- `gog classroom submissions <courseId> <courseworkId> [--state ...] [--max N] [--page TOKEN]`
- `gog classroom submissions get <courseId> <courseworkId> <submissionId>`
- `gog classroom submissions turn-in <courseId> <courseworkId> <submissionId>`
- `gog classroom submissions reclaim <courseId> <courseworkId> <submissionId>`
- `gog classroom submissions return <courseId> <courseworkId> <submissionId>`
- `gog classroom submissions grade <courseId> <courseworkId> <submissionId> [--draft N] [--assigned N]`
- `gog classroom announcements <courseId> [--state ...] [--max N] [--page TOKEN]`
- `gog classroom announcements get <courseId> <announcementId>`
- `gog classroom announcements create <courseId> --text TEXT`
- `gog classroom announcements update <courseId> <announcementId> [--text ...]`
- `gog classroom announcements delete <courseId> <announcementId>`
- `gog classroom announcements assignees <courseId> <announcementId> [--mode ...]`
- `gog classroom topics <courseId> [--max N] [--page TOKEN]`
- `gog classroom topics get <courseId> <topicId>`
- `gog classroom topics create <courseId> --name NAME`
- `gog classroom topics update <courseId> <topicId> --name NAME`
- `gog classroom topics delete <courseId> <topicId>`
- `gog classroom invitations [--course ID] [--user ID]`
- `gog classroom invitations get <invitationId>`
- `gog classroom invitations create <courseId> <userId> --role STUDENT|TEACHER|OWNER`
- `gog classroom invitations accept <invitationId>`
- `gog classroom invitations delete <invitationId>`
- `gog classroom guardians <studentId> [--max N] [--page TOKEN]`
- `gog classroom guardians get <studentId> <guardianId>`
- `gog classroom guardians delete <studentId> <guardianId>`
- `gog classroom guardian-invitations <studentId> [--state ...] [--max N] [--page TOKEN]`
- `gog classroom guardian-invitations get <studentId> <invitationId>`
- `gog classroom guardian-invitations create <studentId> --email EMAIL`
- `gog classroom profile [userId]`
- `gog gmail search <query> [--max N] [--page TOKEN]`
- `gog gmail messages search <query> [--max N] [--page TOKEN] [--include-body]`
- `gog gmail thread get <threadId> [--download]`
- `gog gmail thread modify <threadId> [--add ...] [--remove ...]`
- `gog gmail get <messageId> [--format full|metadata|raw] [--headers ...]`
- `gog gmail attachment <messageId> <attachmentId> [--out PATH] [--name NAME]`
- `gog gmail url <threadIds...>`
- `gog gmail labels list`
- `gog gmail labels get <labelIdOrName>`
- `gog gmail labels create <name>`
- `gog gmail labels modify <threadIds...> [--add ...] [--remove ...]`
- `gog gmail send --to a@b.com --subject S [--body B] [--body-html H] [--cc ...] [--bcc ...] [--reply-to-message-id <messageId>] [--reply-to addr] [--attach <file>...]`
- `gog gmail drafts list [--max N] [--page TOKEN]`
- `gog gmail drafts get <draftId> [--download]`
- `gog gmail drafts create --subject S [--to a@b.com] [--body B] [--body-html H] [--cc ...] [--bcc ...] [--reply-to-message-id <messageId>] [--reply-to addr] [--attach <file>...]`
- `gog gmail drafts update <draftId> --subject S [--to a@b.com] [--body B] [--body-html H] [--cc ...] [--bcc ...] [--reply-to-message-id <messageId>] [--reply-to addr] [--attach <file>...]`
- `gog gmail drafts send <draftId>`
- `gog gmail drafts delete <draftId>`
- `gog gmail watch start|status|renew|stop|serve`
- `gog gmail history --since <historyId>`
- `gog chat spaces list [--max N] [--page TOKEN]`
- `gog chat spaces find <displayName> [--max N]`
- `gog chat spaces create <displayName> [--member email,...]`
- `gog chat messages list <space> [--max N] [--page TOKEN] [--order ORDER] [--thread THREAD] [--unread]`
- `gog chat messages send <space> --text TEXT [--thread THREAD]`
- `gog chat threads list <space> [--max N] [--page TOKEN]`
- `gog chat dm space <email>`
- `gog chat dm send <email> --text TEXT [--thread THREAD]`
- `gog tasks lists [--max N] [--page TOKEN]`
- `gog tasks lists create <title>`
- `gog tasks list <tasklistId> [--max N] [--page TOKEN]`
- `gog tasks get <tasklistId> <taskId>`
- `gog tasks add <tasklistId> --title T [--notes N] [--due RFC3339|YYYY-MM-DD] [--repeat daily|weekly|monthly|yearly] [--repeat-count N] [--repeat-until DT] [--parent ID] [--previous ID]`
- `gog tasks update <tasklistId> <taskId> [--title T] [--notes N] [--due RFC3339|YYYY-MM-DD] [--status needsAction|completed]`
- `gog tasks done <tasklistId> <taskId>`
- `gog tasks undo <tasklistId> <taskId>`
- `gog tasks delete <tasklistId> <taskId>`
- `gog tasks clear <tasklistId>`
- `gog contacts search <query> [--max N]`
- `gog contacts list [--max N] [--page TOKEN]`
- `gog contacts get <people/...|email>`
- `gog contacts create --given NAME [--family NAME] [--email addr] [--phone num]`
- `gog contacts update <people/...> [--given NAME] [--family NAME] [--email addr] [--phone num] [--birthday YYYY-MM-DD] [--notes TEXT] [--from-file PATH|-] [--ignore-etag]`
- `gog contacts delete <people/...>`
- `gog contacts directory list [--max N] [--page TOKEN]`
- `gog contacts directory search <query> [--max N] [--page TOKEN]`
- `gog contacts other list [--max N] [--page TOKEN]`
- `gog contacts other search <query> [--max N]`
- `gog people me`
- `gog people get <people/...|userId>`
- `gog people search <query> [--max N] [--page TOKEN]`
- `gog people relations [<people/...|userId>] [--type TYPE]`

Date/time input conventions (shared parser):

- Date-only: `YYYY-MM-DD`
- Datetime: `RFC3339` / `RFC3339Nano` / `YYYY-MM-DDTHH:MM[:SS]` / `YYYY-MM-DD HH:MM[:SS]`
- Numeric timezone offset accepted: `YYYY-MM-DDTHH:MM:SS-0800`
- Calendar range flags also accept relatives: `now`, `today`, `tomorrow`, `yesterday`, weekday names (`monday`, `next friday`)
- Tracking `--since` also accepts durations like `24h`

### Planned high-level command tree

- `gog auth …`
  - `gog auth credentials <credentials.json>`
  - `gog auth credentials list`
  - `gog --client <name> auth credentials <credentials.json>`
- `gog gmail …`
- `gog chat …`
- `gog calendar …`
- `gog drive …`
- `gog contacts …`
- `gog tasks …`
- `gog people …`

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

- `gog auth add` requests a union of scopes based on `--services`.
- Each API client refreshes an access token for the subset of scopes needed for that service.
- If you later want additional services, re-run `gog auth add <email> --services ...` (may require `--force-consent` to mint a new refresh token).

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

- `cmd/gog/main.go` — binary entrypoint
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
  - stored `credentials.json` (or `credentials-<client>.json`) via `gog auth credentials ...`
  - refresh token in keyring via `gog auth add <email>`
- Run:
  - `GOG_IT_ACCOUNT=you@gmail.com go test -tags=integration ./internal/integration`
  - optional: `GOG_CLIENT=work` to select a non-default OAuth client

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
