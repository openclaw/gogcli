# 🧭 ratatosk — Google in your terminal.

![GitHub Repo Banner](https://ghrb.waren.build/banner?header=ratatosk%F0%9F%A7%AD&subheader=Google+in+your+terminal&bg=f3f4f6&color=1f2937&support=true)
<!-- Created with GitHub Repo Banner by Waren Gonzaga: https://ghrb.waren.build -->

Fast, script-friendly CLI for Gmail, Calendar, Chat, Classroom, Drive, Docs, Slides, Sheets, Contacts, Tasks, People, Groups (Workspace), and Keep (Workspace-only). JSON-first output, multiple accounts, and least-privilege auth built in.

## Fork Status

This is a CampusIQ-maintained fork of [steipete/gogcli](https://github.com/steipete/gogcli).

**Why:** Upstream has limited merge activity (30+ open PRs, batch review cycles). We needed Google Docs write/update commands and Drive/Docs comments support on our timeline.

**Strategy:** Hard fork — we own this fully under the `ratatosk` identity (`rata` binary). Module path is `github.com/degree-analytics/ratatosk`. We keep `upstream` remote for future cherry-picking.

**Distribution:** Via Homebrew tap (`brew install degree-analytics/tap/ratatosk`).

**Upstream:** PR #185 remains open as a contribution. We have no dependency on it being merged.

## Features

- **Gmail** - search threads and messages, send emails, view attachments, manage labels/drafts/filters/delegation/vacation settings, history, and watch (Pub/Sub push)
- **Email tracking** - track opens for `rata gmail send --track` with a small Cloudflare Worker backend
- **Calendar** - list/create/update events, detect conflicts, manage invitations, check free/busy status, team calendars, propose new times, focus/OOO/working-location events, recurrence + reminders
- **Classroom** - manage courses, roster, coursework/materials, submissions, announcements, topics, invitations, guardians, profiles
- **Chat** - list/find/create spaces, list messages/threads (filter by thread/unread), send messages and DMs (Workspace-only)
- **Drive** - list/search/upload/download files, manage permissions/comments, organize folders, list shared drives
- **Contacts** - search/create/update contacts, access Workspace directory/other contacts
- **Tasks** - manage tasklists and tasks: get/create/add/update/done/undo/delete/clear, repeat schedules
- **Sheets** - read/write/update spreadsheets, format cells, create new sheets (and export via Drive)
- **Docs/Slides** - export to PDF/DOCX/PPTX via Drive (plus create/copy, docs-to-text)
- **People** - access profile information
- **Keep (Workspace only)** - list/get/search notes and download attachments (service account + domain-wide delegation)
- **Groups** - list groups you belong to, view group members (Google Workspace)
- **Local time** - quick local/UTC time display for scripts and agents
- **Multiple accounts** - manage multiple Google accounts simultaneously (with aliases)
- **Command allowlist** - restrict top-level commands for sandboxed/agent runs
- **Secure credential storage** using OS keyring or encrypted on-disk keyring (configurable)
- **Auto-refreshing tokens** - authenticate once, use indefinitely
- **Least-privilege auth** - `--readonly` and `--drive-scope` to request fewer scopes
- **Workspace service accounts** - domain-wide delegation auth (preferred when configured)
- **Parseable output** - JSON mode for scripting and automation (Calendar adds day-of-week fields)

## Installation

### Homebrew

```bash
brew install degree-analytics/tap/ratatosk
```

### Build from Source

```bash
git clone https://github.com/degree-analytics/ratatosk.git
cd ratatosk
make
```

Run:

```bash
./bin/rata --help
```

Help:

- `rata --help` shows top-level command groups.
- Drill down with `rata <group> --help` (and deeper subcommands).
- For the full expanded command list: `RATA_HELP=full rata --help`.
- Make shortcut: `make rata -- --help` (or `make rata -- gmail --help`).
- `make rata-help` shows CLI help (note: `make rata --help` is Make's own help; use `--`).

## Quick Start

### 1. Get OAuth2 Credentials

Before adding an account, create OAuth2 credentials from Google Cloud Console:

1. Open the Google Cloud Console credentials page: https://console.cloud.google.com/apis/credentials
1. Create a project: https://console.cloud.google.com/projectcreate
2. Enable the APIs you need:
   - Gmail API: https://console.cloud.google.com/apis/api/gmail.googleapis.com
   - Google Calendar API: https://console.cloud.google.com/apis/api/calendar-json.googleapis.com
   - Google Chat API: https://console.cloud.google.com/apis/api/chat.googleapis.com
   - Google Drive API: https://console.cloud.google.com/apis/api/drive.googleapis.com
   - Google Classroom API: https://console.cloud.google.com/apis/api/classroom.googleapis.com
   - People API (Contacts): https://console.cloud.google.com/apis/api/people.googleapis.com
   - Google Tasks API: https://console.cloud.google.com/apis/api/tasks.googleapis.com
   - Google Sheets API: https://console.cloud.google.com/apis/api/sheets.googleapis.com
   - Cloud Identity API (Groups): https://console.cloud.google.com/apis/api/cloudidentity.googleapis.com
3. Configure OAuth consent screen: https://console.cloud.google.com/auth/branding
4. If your app is in "Testing", add test users: https://console.cloud.google.com/auth/audience
5. Create OAuth client:
   - Go to https://console.cloud.google.com/auth/clients
   - Click "Create Client"
   - Application type: "Desktop app"
   - Download the JSON file (usually named `client_secret_....apps.googleusercontent.com.json`)

### 2. Store Credentials

```bash
rata auth credentials ~/Downloads/client_secret_....json
```

For multiple OAuth clients/projects:

```bash
rata --client work auth credentials ~/Downloads/work-client.json
rata auth credentials list
```

### 3. Authorize Your Account

```bash
rata auth add you@gmail.com
```

This will open a browser window for OAuth authorization. The refresh token is stored securely in your system keychain.

### 4. Test Authentication

```bash
export RATA_ACCOUNT=you@gmail.com
rata gmail labels list
```

## Authentication & Secrets

### Accounts and tokens

`rata` stores your OAuth refresh tokens in a "keyring" backend. Default is `auto` (best available backend for your OS/environment).

Before you can run `rata auth add`, you must store OAuth client credentials once via `rata auth credentials <credentials.json>` (download a Desktop app OAuth client JSON from the Cloud Console). For multiple clients, use `rata --client <name> auth credentials ...`; tokens are isolated per client.

List accounts:

```bash
rata auth list
```

Verify tokens are usable (helps spot revoked/expired tokens):

```bash
rata auth list --check
```

Accounts can be authorized either via OAuth refresh tokens or Workspace service accounts (domain-wide delegation). If a service account key is configured for an account, it takes precedence over OAuth refresh tokens (see `rata auth list`).

Show current auth state/services for the active account:

```bash
rata auth status
```

### Multiple OAuth clients

Use `--client` (or `RATA_CLIENT`) to select a named OAuth client:

```bash
rata --client work auth credentials ~/Downloads/work.json
rata --client work auth add you@company.com
```

Optional domain mapping for auto-selection:

```bash
rata --client work auth credentials ~/Downloads/work.json --domain example.com
```

How it works:

- Default client is `default` (stored in `credentials.json`).
- Named clients are stored as `credentials-<client>.json`.
- Tokens are isolated per client (`token:<client>:<email>`); defaults are per client too.

Client selection order (when `--client` is not set):

1) `--client` / `RATA_CLIENT`
2) `account_clients` config (email -> client)
3) `client_domains` config (domain -> client)
4) Credentials file named after the email domain (`credentials-example.com.json`)
5) `default`

Config example (JSON5):

```json5
{
  account_clients: { "you@company.com": "work" },
  client_domains: { "example.com": "work" },
}
```

List stored credentials:

```bash
rata auth credentials list
```

See `docs/auth-clients.md` for the full client selection and mapping rules.

### Keyring backend: Keychain vs encrypted file

Backends:

- `auto` (default): picks the best backend for the platform.
- `keychain`: macOS Keychain (recommended on macOS; avoids password management).
- `file`: encrypted on-disk keyring (requires a password).

Set backend via command (writes `keyring_backend` into `config.json`):

```bash
rata auth keyring file
rata auth keyring keychain
rata auth keyring auto
```

Show current backend + source (env/config/default) and config path:

```bash
rata auth keyring
```

Non-interactive runs (CI/ssh): file backend requires `RATA_KEYRING_PASSWORD`.

```bash
export RATA_KEYRING_PASSWORD='...'
rata --no-input auth status
```

Force backend via env (overrides config):

```bash
export RATA_KEYRING_BACKEND=file
```

Precedence: `RATA_KEYRING_BACKEND` env var overrides `config.json`.

## Configuration

### Account Selection

Specify the account using either a flag or environment variable:

```bash
# Via flag
rata gmail search 'newer_than:7d' --account you@gmail.com

# Via alias
rata auth alias set work work@company.com
rata gmail search 'newer_than:7d' --account work

# Via environment
export RATA_ACCOUNT=you@gmail.com
rata gmail search 'newer_than:7d'

# Auto-select (default account or the single stored token)
rata gmail labels list --account auto
```

List configured accounts:

```bash
rata auth list
```

### Output

- Default: human-friendly tables on stdout.
- `--plain`: stable TSV on stdout (tabs preserved; best for piping to tools that expect `\t`).
- `--json`: JSON on stdout (best for scripting).
- Human-facing hints/progress go to stderr.
- Colors are enabled only in rich TTY output and are disabled automatically for `--json` and `--plain`.

### Service Scopes

By default, `rata auth add` requests access to the **user** services (see `rata auth services` for the current list and scopes).

To request fewer scopes:

```bash
rata auth add you@gmail.com --services drive,calendar
```

To request read-only scopes (write operations will fail with 403 insufficient scopes):

```bash
rata auth add you@gmail.com --services drive,calendar --readonly
```

To control Drive's scope (default: `full`):

```bash
rata auth add you@gmail.com --services drive --drive-scope full
rata auth add you@gmail.com --services drive --drive-scope readonly
rata auth add you@gmail.com --services drive --drive-scope file
```

Notes:

- `--drive-scope readonly` is enough for listing/downloading/exporting via Drive (write operations will 403).
- `--drive-scope file` is write-capable (limited to files created/opened by this app) and can't be combined with `--readonly`.

If you need to add services later and Google doesn't return a refresh token, re-run with `--force-consent`:

```bash
rata auth add you@gmail.com --services user --force-consent
# Or add just Sheets
rata auth add you@gmail.com --services sheets --force-consent
```

`--services all` is accepted as an alias for `user` for backwards compatibility.

Docs commands are implemented via the Drive API, and `docs` requests both Drive and Docs API scopes.

Service scope matrix (auto-generated; run `go run scripts/gen-auth-services-md.go`):

<!-- auth-services:start -->
| Service | User | APIs | Scopes | Notes |
| --- | --- | --- | --- | --- |
| gmail | yes | Gmail API | `https://www.googleapis.com/auth/gmail.modify`<br>`https://www.googleapis.com/auth/gmail.settings.basic`<br>`https://www.googleapis.com/auth/gmail.settings.sharing` |  |
| calendar | yes | Calendar API | `https://www.googleapis.com/auth/calendar` |  |
| chat | yes | Chat API | `https://www.googleapis.com/auth/chat.spaces`<br>`https://www.googleapis.com/auth/chat.messages`<br>`https://www.googleapis.com/auth/chat.memberships`<br>`https://www.googleapis.com/auth/chat.users.readstate.readonly` |  |
| classroom | yes | Classroom API | `https://www.googleapis.com/auth/classroom.courses`<br>`https://www.googleapis.com/auth/classroom.rosters`<br>`https://www.googleapis.com/auth/classroom.coursework.students`<br>`https://www.googleapis.com/auth/classroom.coursework.me`<br>`https://www.googleapis.com/auth/classroom.courseworkmaterials`<br>`https://www.googleapis.com/auth/classroom.announcements`<br>`https://www.googleapis.com/auth/classroom.topics`<br>`https://www.googleapis.com/auth/classroom.guardianlinks.students`<br>`https://www.googleapis.com/auth/classroom.profile.emails`<br>`https://www.googleapis.com/auth/classroom.profile.photos` |  |
| drive | yes | Drive API | `https://www.googleapis.com/auth/drive` |  |
| docs | yes | Docs API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/documents` | Export/copy/create via Drive |
| contacts | yes | People API | `https://www.googleapis.com/auth/contacts`<br>`https://www.googleapis.com/auth/contacts.other.readonly`<br>`https://www.googleapis.com/auth/directory.readonly` | Contacts + other contacts + directory |
| tasks | yes | Tasks API | `https://www.googleapis.com/auth/tasks` |  |
| sheets | yes | Sheets API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/spreadsheets` | Export via Drive |
| people | yes | People API | `profile` | OIDC profile scope |
| groups | no | Cloud Identity API | `https://www.googleapis.com/auth/cloud-identity.groups.readonly` | Workspace only |
| keep | no | Keep API | `https://www.googleapis.com/auth/keep.readonly` | Workspace only; service account (domain-wide delegation) |
<!-- auth-services:end -->

### Service Accounts (Workspace only)

A service account is a non-human Google identity that belongs to a Google Cloud project. In Google Workspace, a service account can impersonate a user via **domain-wide delegation** (admin-controlled) and access APIs like Gmail/Calendar/Drive as that user.

In `rata`, service accounts are an **optional auth method** that can be configured per account email. If a service account key is configured for an account, it takes precedence over OAuth refresh tokens (see `rata auth list`).

#### 1) Create a Service Account (Google Cloud)

1. Create (or pick) a Google Cloud project.
2. Enable the APIs you'll use (e.g. Gmail, Calendar, Drive, Sheets, Docs, People, Tasks, Cloud Identity).
3. Go to **IAM & Admin → Service Accounts** and create a service account.
4. In the service account details, enable **Domain-wide delegation**.
5. Create a key (**Keys → Add key → Create new key → JSON**) and download the JSON key file.

#### 2) Allowlist scopes (Google Workspace Admin Console)

Domain-wide delegation is enforced by Workspace admin settings.

1. Open **Admin console → Security → API controls → Domain-wide delegation**.
2. Add a new API client:
   - Client ID: use the service account's "Client ID" from Google Cloud.
   - OAuth scopes: comma-separated list of scopes you want to allow (copy from `rata auth services` and/or your `rata auth add --services ...` usage).

If a scope is missing from the allowlist, service-account token minting can fail (or API calls will 403 with insufficient permissions).

#### 3) Configure `rata` to use the service account

Store the key for the user you want to impersonate:

```bash
rata auth service-account set you@yourdomain.com --key ~/Downloads/service-account.json
```

Verify `rata` is preferring the service account for that account:

```bash
rata --account you@yourdomain.com auth status
rata auth list
```

### Google Keep (Workspace only)

Keep requires Workspace + domain-wide delegation. You can configure it via the generic service-account command above (recommended), or the legacy Keep helper:

```bash
rata auth service-account set you@yourdomain.com --key ~/Downloads/service-account.json
rata keep list --account you@yourdomain.com
rata keep get <noteId> --account you@yourdomain.com
```

### Environment Variables

- `RATA_ACCOUNT` - Default account email or alias to use (avoids repeating `--account`; otherwise uses keyring default or a single stored token)
- `RATA_CLIENT` - OAuth client name (selects stored credentials + token bucket)
- `RATA_JSON` - Default JSON output
- `RATA_PLAIN` - Default plain output
- `RATA_COLOR` - Color mode: `auto` (default), `always`, or `never`
- `RATA_TIMEZONE` - Default output timezone for Calendar/Gmail (IANA name, `UTC`, or `local`)
- `RATA_ENABLE_COMMANDS` - Comma-separated allowlist of top-level commands (e.g., `calendar,tasks`)

### Config File (JSON5)

Find the actual config path in `rata --help` or `rata auth keyring`.

Typical paths:

- macOS: `~/Library/Application Support/ratatosk/config.json`
- Linux: `~/.config/ratatosk/config.json` (or `$XDG_CONFIG_HOME/ratatosk/config.json`)
- Windows: `%AppData%\\ratatosk\\config.json`

Example (JSON5 supports comments and trailing commas):

```json5
{
  // Avoid macOS Keychain prompts
  keyring_backend: "file",
  // Default output timezone for Calendar/Gmail (IANA, UTC, or local)
  default_timezone: "UTC",
  // Optional account aliases
  account_aliases: {
    work: "work@company.com",
    personal: "me@gmail.com",
  },
  // Optional per-account OAuth client selection
  account_clients: {
    "work@company.com": "work",
  },
  // Optional domain -> client mapping
  client_domains: {
    "example.com": "work",
  },
}
```

### Config Commands

```bash
rata config path
rata config list
rata config keys
rata config get default_timezone
rata config set default_timezone UTC
rata config unset default_timezone
```

### Account Aliases

```bash
rata auth alias set work work@company.com
rata auth alias list
rata auth alias unset work
```

Aliases work anywhere you pass `--account` or `RATA_ACCOUNT` (reserved: `auto`, `default`).

### Command Allowlist (Sandboxing)

```bash
# Only allow calendar + tasks commands for an agent
rata --enable-commands calendar,tasks calendar events --today

# Same via env
export RATA_ENABLE_COMMANDS=calendar,tasks
rata tasks list <tasklistId>
```

## Security

### Credential Storage

OAuth credentials are stored securely in your system's keychain:
- **macOS**: Keychain Access
- **Linux**: Secret Service (GNOME Keyring, KWallet)
- **Windows**: Credential Manager

The CLI uses [github.com/99designs/keyring](https://github.com/99designs/keyring) for secure storage.

If no OS keychain backend is available (e.g., Linux/WSL/container), keyring can fall back to an encrypted on-disk store and may prompt for a password; for non-interactive runs set `RATA_KEYRING_PASSWORD`.

### Keychain Prompts (macOS)

macOS Keychain may prompt more than you'd expect when the "app identity" keeps changing (different binary path, `go run` temp builds, rebuilding to new `./bin/rata`, multiple copies). Keychain treats those as different apps, so it asks again.

Options:

- **Default (recommended):** keep using Keychain (secure) and run a stable `rata` binary path to reduce repeat prompts.
- **Force Keychain:** `RATA_KEYRING_BACKEND=keychain` (disables any file-backend fallback).
- **Avoid Keychain prompts entirely:** `RATA_KEYRING_BACKEND=file` (stores encrypted entries on disk under your config dir).
  - To avoid password prompts too (CI/non-interactive): set `RATA_KEYRING_PASSWORD=...` (tradeoff: secret in env).

### Best Practices

- **Never commit OAuth client credentials** to version control
- Store client credentials outside your project directory
- Use different OAuth clients for development and production
- Re-authorize with `--force-consent` if you suspect token compromise
- Remove unused accounts with `rata auth remove <email>`

## Commands

Flag aliases:
- `--out` also accepts `--output`.
- `--out-dir` also accepts `--output-dir` (Gmail thread attachment downloads).

### Authentication

```bash
rata auth credentials <path>           # Store OAuth client credentials
rata auth credentials list             # List stored OAuth client credentials
rata --client work auth credentials <path>  # Store named OAuth client credentials
rata auth add <email>                  # Authorize and store refresh token
rata auth service-account set <email> --key <path>  # Configure service account impersonation (Workspace only)
rata auth service-account status <email>            # Show service account status
rata auth service-account unset <email>             # Remove service account
rata auth keep <email> --key <path>                 # Legacy alias (Keep)
rata auth keyring [backend]            # Show/set keyring backend (auto|keychain|file)
rata auth status                       # Show current auth state/services
rata auth services                     # List available services and OAuth scopes
rata auth list                         # List stored accounts
rata auth list --check                 # Validate stored refresh tokens
rata auth remove <email>               # Remove a stored refresh token
rata auth manage                       # Open accounts manager in browser
rata auth tokens                       # Manage stored refresh tokens
```

### Keep (Workspace only)

```bash
rata keep list --account you@yourdomain.com
rata keep get <noteId> --account you@yourdomain.com
rata keep search <query> --account you@yourdomain.com
rata keep attachment <attachmentName> --account you@yourdomain.com --out ./attachment.bin
```

### Gmail

```bash
# Search and read
rata gmail search 'newer_than:7d' --max 10
rata gmail thread get <threadId>
rata gmail thread get <threadId> --download              # Download attachments to current dir
rata gmail thread get <threadId> --download --out-dir ./attachments
rata gmail get <messageId>
rata gmail get <messageId> --format metadata
rata gmail attachment <messageId> <attachmentId>
rata gmail attachment <messageId> <attachmentId> --out ./attachment.bin
rata gmail url <threadId>              # Print Gmail web URL
rata gmail thread modify <threadId> --add STARRED --remove INBOX

# Send and compose
rata gmail send --to a@b.com --subject "Hi" --body "Plain fallback"
rata gmail send --to a@b.com --subject "Hi" --body-file ./message.txt
rata gmail send --to a@b.com --subject "Hi" --body-file -   # Read body from stdin
rata gmail send --to a@b.com --subject "Hi" --body "Plain fallback" --body-html "<p>Hello</p>"
rata gmail drafts list
rata gmail drafts create --subject "Draft" --body "Body"
rata gmail drafts create --to a@b.com --subject "Draft" --body "Body"
rata gmail drafts update <draftId> --subject "Draft" --body "Body"
rata gmail drafts update <draftId> --to a@b.com --subject "Draft" --body "Body"
rata gmail drafts send <draftId>

# Labels
rata gmail labels list
rata gmail labels get INBOX --json  # Includes message counts
rata gmail labels create "My Label"
rata gmail labels modify <threadId> --add STARRED --remove INBOX

# Batch operations
rata gmail batch delete <messageId> <messageId>
rata gmail batch modify <messageId> <messageId> --add STARRED --remove INBOX

# Filters
rata gmail filters list
rata gmail filters create --from 'noreply@example.com' --add-label 'Notifications'
rata gmail filters delete <filterId>

# Settings
rata gmail autoforward get
rata gmail autoforward enable --email forward@example.com
rata gmail autoforward disable
rata gmail forwarding list
rata gmail forwarding add --email forward@example.com
rata gmail sendas list
rata gmail sendas create --email alias@example.com
rata gmail vacation get
rata gmail vacation enable --subject "Out of office" --message "..."
rata gmail vacation disable

# Delegation (G Suite/Workspace)
rata gmail delegates list
rata gmail delegates add --email delegate@example.com
rata gmail delegates remove --email delegate@example.com

# Watch (Pub/Sub push)
rata gmail watch start --topic projects/<p>/topics/<t> --label INBOX
rata gmail watch serve --bind 127.0.0.1 --token <shared> --hook-url http://127.0.0.1:18789/hooks/agent
rata gmail watch serve --bind 0.0.0.0 --verify-oidc --oidc-email <svc@...> --hook-url <url>
rata gmail history --since <historyId>
```

Gmail watch (Pub/Sub push):
- Create Pub/Sub topic + push subscription (OIDC preferred; shared token ok for dev).
- Full flow + payload details: `docs/watch.md`.

### Email Tracking

Track when recipients open your emails:

```bash
# Set up local tracking config (per-account; generates keys; follow printed deploy steps)
rata gmail track setup --worker-url https://rata-email-tracker.<acct>.workers.dev

# Send with tracking
rata gmail send --to recipient@example.com --subject "Hello" --body-html "<p>Hi!</p>" --track

# Check opens
rata gmail track opens <tracking_id>
rata gmail track opens --to recipient@example.com

# View status
rata gmail track status
```

Docs: `docs/email-tracking.md` (setup/deploy) + `docs/email-tracking-worker.md` (internals).

**Notes:** `--track` requires exactly 1 recipient (no cc/bcc) and an HTML body (`--body-html`). Use `--track-split` to send per-recipient messages with individual tracking ids. The tracking worker stores IP/user-agent + coarse geo by default.

### Calendar

```bash
# Calendars
rata calendar calendars
rata calendar acl <calendarId>         # List access control rules
rata calendar colors                   # List available event/calendar colors
rata calendar time --timezone America/New_York
rata calendar users                    # List workspace users (use email as calendar ID)

# Events (with timezone-aware time flags)
rata calendar events <calendarId> --today                    # Today's events
rata calendar events <calendarId> --tomorrow                 # Tomorrow's events
rata calendar events <calendarId> --week                     # This week (Mon-Sun by default; use --week-start)
rata calendar events <calendarId> --days 3                   # Next 3 days
rata calendar events <calendarId> --from today --to friday   # Relative dates
rata calendar events <calendarId> --from today --to friday --weekday   # Include weekday columns
rata calendar events <calendarId> --from 2025-01-01T00:00:00Z --to 2025-01-08T00:00:00Z
rata calendar events --all             # Fetch events from all calendars
rata calendar event <calendarId> <eventId>
rata calendar get <calendarId> <eventId>                     # Alias for event
rata calendar search "meeting" --today
rata calendar search "meeting" --tomorrow
rata calendar search "meeting" --days 365
rata calendar search "meeting" --from 2025-01-01T00:00:00Z --to 2025-01-31T00:00:00Z --max 50

# Search defaults to 30 days ago through 90 days ahead unless you set --from/--to/--today/--week/--days.
# Tip: set RATA_CALENDAR_WEEKDAY=1 to default --weekday for calendar events output.

# JSON event output includes timezone and localized times (useful for agents).
rata calendar get <calendarId> <eventId> --json
# {
#   "event": {
#     "id": "...",
#     "summary": "...",
#     "startDayOfWeek": "Friday",
#     "endDayOfWeek": "Friday",
#     "timezone": "America/Los_Angeles",
#     "eventTimezone": "America/New_York",
#     "startLocal": "2026-01-23T20:45:00-08:00",
#     "endLocal": "2026-01-23T22:45:00-08:00",
#     "start": { "dateTime": "2026-01-23T23:45:00-05:00" },
#     "end": { "dateTime": "2026-01-24T01:45:00-05:00" }
#   }
# }

# Team calendars (requires Cloud Identity API for Google Workspace)
rata calendar team <group-email> --today           # Show team's events for today
rata calendar team <group-email> --week            # Show team's events for the week (use --week-start)
rata calendar team <group-email> --freebusy        # Show only busy/free blocks (faster)
rata calendar team <group-email> --query "standup" # Filter by event title

# Create and update
rata calendar create <calendarId> \
  --summary "Meeting" \
  --from 2025-01-15T10:00:00Z \
  --to 2025-01-15T11:00:00Z

rata calendar create <calendarId> \
  --summary "Team Sync" \
  --from 2025-01-15T14:00:00Z \
  --to 2025-01-15T15:00:00Z \
  --attendees "alice@example.com,bob@example.com" \
  --location "Zoom"

rata calendar update <calendarId> <eventId> \
  --summary "Updated Meeting" \
  --from 2025-01-15T11:00:00Z \
  --to 2025-01-15T12:00:00Z

# Send notifications when creating/updating
rata calendar create <calendarId> \
  --summary "Team Sync" \
  --from 2025-01-15T14:00:00Z \
  --to 2025-01-15T15:00:00Z \
  --send-updates all

rata calendar update <calendarId> <eventId> \
  --send-updates externalOnly

# Recurrence + reminders
rata calendar create <calendarId> \
  --summary "Payment" \
  --from 2025-02-11T09:00:00-03:00 \
  --to 2025-02-11T09:15:00-03:00 \
  --rrule "RRULE:FREQ=MONTHLY;BYMONTHDAY=11" \
  --reminder "email:3d" \
  --reminder "popup:30m"

# Special event types via --event-type (focus-time/out-of-office/working-location)
rata calendar create primary \
  --event-type focus-time \
  --from 2025-01-15T13:00:00Z \
  --to 2025-01-15T14:00:00Z

rata calendar create primary \
  --event-type out-of-office \
  --from 2025-01-20 \
  --to 2025-01-21 \
  --all-day

rata calendar create primary \
  --event-type working-location \
  --working-location-type office \
  --working-office-label "HQ" \
  --from 2025-01-22 \
  --to 2025-01-23

# Dedicated shortcuts (same event types, more opinionated defaults)
rata calendar focus-time --from 2025-01-15T13:00:00Z --to 2025-01-15T14:00:00Z
rata calendar out-of-office --from 2025-01-20 --to 2025-01-21 --all-day
rata calendar working-location --type office --office-label "HQ" --from 2025-01-22 --to 2025-01-23
# Add attendees without replacing existing attendees/RSVP state
rata calendar update <calendarId> <eventId> \
  --add-attendee "alice@example.com,bob@example.com"

rata calendar delete <calendarId> <eventId>

# Invitations
rata calendar respond <calendarId> <eventId> --status accepted
rata calendar respond <calendarId> <eventId> --status declined
rata calendar respond <calendarId> <eventId> --status tentative
rata calendar respond <calendarId> <eventId> --status declined --send-updates externalOnly

# Propose a new time (browser-only flow; API limitation)
rata calendar propose-time <calendarId> <eventId>
rata calendar propose-time <calendarId> <eventId> --open
rata calendar propose-time <calendarId> <eventId> --decline --comment "Can we do 5pm?"

# Availability
rata calendar freebusy --calendars "primary,work@example.com" \
  --from 2025-01-15T00:00:00Z \
  --to 2025-01-16T00:00:00Z

rata calendar conflicts --calendars "primary,work@example.com" \
  --today                             # Today's conflicts
```

### Time

```bash
rata time now
rata time now --timezone UTC
```

### Drive

```bash
# List and search
rata drive ls --max 20
rata drive ls --parent <folderId> --max 20
rata drive search "invoice" --max 20
rata drive get <fileId>                # Get file metadata
rata drive url <fileId>                # Print Drive web URL
rata drive copy <fileId> "Copy Name"

# Upload and download
rata drive upload ./path/to/file --parent <folderId>
rata drive upload ./path/to/report.docx --convert
rata drive download <fileId> --out ./downloaded.bin
rata drive download <fileId> --format pdf --out ./exported.pdf
rata drive download <fileId> --format docx --out ./doc.docx
rata drive download <fileId> --format pptx --out ./slides.pptx

# Organize
rata drive mkdir "New Folder"
rata drive mkdir "New Folder" --parent <parentFolderId>
rata drive rename <fileId> "New Name"
rata drive move <fileId> --parent <destinationFolderId>
rata drive delete <fileId>             # Move to trash

# Permissions
rata drive permissions <fileId>
rata drive share <fileId> --email user@example.com --role reader
rata drive share <fileId> --email user@example.com --role writer
rata drive unshare <fileId> --permission-id <permissionId>

# Shared drives (Team Drives)
rata drive drives --max 100
```

### Docs / Slides / Sheets

```bash
# Docs
rata docs info <docId>
rata docs cat <docId> --max-bytes 10000
rata docs create "My Doc"
rata docs copy <docId> "My Doc Copy"
rata docs export <docId> --format pdf --out ./doc.pdf

# Slides
rata slides info <presentationId>
rata slides create "My Deck"
rata slides copy <presentationId> "My Deck Copy"
rata slides export <presentationId> --format pdf --out ./deck.pdf

# Sheets
rata sheets copy <spreadsheetId> "My Sheet Copy"
rata sheets export <spreadsheetId> --format pdf --out ./sheet.pdf
rata sheets format <spreadsheetId> 'Sheet1!A1:B2' --format-json '{"textFormat":{"bold":true}}' --format-fields 'userEnteredFormat.textFormat.bold'
```

### Contacts

```bash
# Personal contacts
rata contacts list --max 50
rata contacts search "Ada" --max 50
rata contacts get people/<resourceName>
rata contacts get user@example.com     # Get by email

# Other contacts (people you've interacted with)
rata contacts other list --max 50
rata contacts other search "John" --max 50

# Create and update
rata contacts create \
  --given-name "John" \
  --family-name "Doe" \
  --email "john@example.com" \
  --phone "+1234567890"

rata contacts update people/<resourceName> \
  --given-name "Jane" \
  --email "jane@example.com"

rata contacts delete people/<resourceName>

# Workspace directory (requires Google Workspace)
rata contacts directory list --max 50
rata contacts directory search "Jane" --max 50
```

### Tasks

```bash
# Task lists
rata tasks lists --max 50
rata tasks lists create <title>

# Tasks in a list
rata tasks list <tasklistId> --max 50
rata tasks get <tasklistId> <taskId>
rata tasks add <tasklistId> --title "Task title"
rata tasks add <tasklistId> --title "Weekly sync" --due 2025-02-01 --repeat weekly --repeat-count 4
rata tasks add <tasklistId> --title "Daily standup" --due 2025-02-01 --repeat daily --repeat-until 2025-02-05
rata tasks update <tasklistId> <taskId> --title "New title"
rata tasks done <tasklistId> <taskId>
rata tasks undo <tasklistId> <taskId>
rata tasks delete <tasklistId> <taskId>
rata tasks clear <tasklistId>

# Note: Google Tasks treats due dates as date-only; time components may be ignored.
```

### Sheets

```bash
# Read
rata sheets metadata <spreadsheetId>
rata sheets get <spreadsheetId> 'Sheet1!A1:B10'

# Export (via Drive)
rata sheets export <spreadsheetId> --format pdf --out ./sheet.pdf
rata sheets export <spreadsheetId> --format xlsx --out ./sheet.xlsx

# Write
rata sheets update <spreadsheetId> 'A1' 'val1|val2,val3|val4'
rata sheets update <spreadsheetId> 'A1' --values-json '[["a","b"],["c","d"]]'
rata sheets update <spreadsheetId> 'Sheet1!A1:C1' 'new|row|data' --copy-validation-from 'Sheet1!A2:C2'
rata sheets append <spreadsheetId> 'Sheet1!A:C' 'new|row|data'
rata sheets append <spreadsheetId> 'Sheet1!A:C' 'new|row|data' --copy-validation-from 'Sheet1!A2:C2'
rata sheets clear <spreadsheetId> 'Sheet1!A1:B10'

# Format
rata sheets format <spreadsheetId> 'Sheet1!A1:B2' --format-json '{"textFormat":{"bold":true}}' --format-fields 'userEnteredFormat.textFormat.bold'

# Create
rata sheets create "My New Spreadsheet" --sheets "Sheet1,Sheet2"
```

### People

```bash
# Profile
rata people me
rata people get people/<userId>

# Search the Workspace directory
rata people search "Ada Lovelace" --max 5

# Relations (defaults to people/me)
rata people relations
rata people relations people/<userId> --type manager
```

### Chat

```bash
# Spaces
rata chat spaces list
rata chat spaces find "Engineering"
rata chat spaces create "Engineering" --member alice@company.com --member bob@company.com

# Messages
rata chat messages list spaces/<spaceId> --max 5
rata chat messages list spaces/<spaceId> --thread <threadId>
rata chat messages list spaces/<spaceId> --unread
rata chat messages send spaces/<spaceId> --text "Build complete!" --thread spaces/<spaceId>/threads/<threadId>

# Threads
rata chat threads list spaces/<spaceId>

# Direct messages
rata chat dm space user@company.com
rata chat dm send user@company.com --text "ping"
```

Note: Chat commands require a Google Workspace account (consumer @gmail.com accounts are not supported).

### Groups (Google Workspace)

```bash
# List groups you belong to
rata groups list

# List members of a group
rata groups members engineering@company.com
```

Note: Groups commands require the Cloud Identity API and the `cloud-identity.groups.readonly` scope. If you get a permissions error, re-authenticate:

```bash
rata auth add your@email.com --services groups --force-consent
```

### Classroom (Google Workspace for Education)

```bash
# Courses
rata classroom courses list
rata classroom courses list --role teacher
rata classroom courses get <courseId>
rata classroom courses create --name "Math 101"
rata classroom courses update <courseId> --name "Math 102"
rata classroom courses archive <courseId>
rata classroom courses unarchive <courseId>
rata classroom courses url <courseId>

# Roster
rata classroom roster <courseId>
rata classroom roster <courseId> --students
rata classroom students add <courseId> <userId>
rata classroom teachers add <courseId> <userId>

# Coursework
rata classroom coursework list <courseId>
rata classroom coursework get <courseId> <courseworkId>
rata classroom coursework create <courseId> --title "Homework 1" --type ASSIGNMENT --state PUBLISHED
rata classroom coursework update <courseId> <courseworkId> --title "Updated"
rata classroom coursework assignees <courseId> <courseworkId> --mode INDIVIDUAL_STUDENTS --add-student <studentId>

# Materials
rata classroom materials list <courseId>
rata classroom materials create <courseId> --title "Syllabus" --state PUBLISHED

# Submissions
rata classroom submissions list <courseId> <courseworkId>
rata classroom submissions get <courseId> <courseworkId> <submissionId>
rata classroom submissions grade <courseId> <courseworkId> <submissionId> --grade 85
rata classroom submissions return <courseId> <courseworkId> <submissionId>
rata classroom submissions turn-in <courseId> <courseworkId> <submissionId>
rata classroom submissions reclaim <courseId> <courseworkId> <submissionId>

# Announcements
rata classroom announcements list <courseId>
rata classroom announcements create <courseId> --text "Welcome!"
rata classroom announcements update <courseId> <announcementId> --text "Updated"
rata classroom announcements assignees <courseId> <announcementId> --mode INDIVIDUAL_STUDENTS --add-student <studentId>

# Topics
rata classroom topics list <courseId>
rata classroom topics create <courseId> --name "Unit 1"
rata classroom topics update <courseId> <topicId> --name "Unit 2"

# Invitations
rata classroom invitations list
rata classroom invitations create <courseId> <userId> --role student
rata classroom invitations accept <invitationId>

# Guardians
rata classroom guardians list <studentId>
rata classroom guardians get <studentId> <guardianId>
rata classroom guardians delete <studentId> <guardianId>

# Guardian invitations
rata classroom guardian-invitations list <studentId>
rata classroom guardian-invitations create <studentId> --email parent@example.com

# Profiles
rata classroom profile get
rata classroom profile get <userId>
```

Note: Classroom commands require a Google Workspace for Education account. Personal Google accounts have limited Classroom functionality.

### Docs

```bash
# Export (via Drive)
rata docs export <docId> --format pdf --out ./doc.pdf
rata docs export <docId> --format docx --out ./doc.docx
rata docs export <docId> --format txt --out ./doc.txt
```

### Slides

```bash
# Export (via Drive)
rata slides export <presentationId> --format pptx --out ./deck.pptx
rata slides export <presentationId> --format pdf --out ./deck.pdf
```

## Output Formats

### Text

Human-readable output with colors (default):

```bash
$ rata gmail search 'newer_than:7d' --max 3
THREAD_ID           SUBJECT                           FROM                  DATE
18f1a2b3c4d5e6f7    Meeting notes                     alice@example.com     2025-01-10
17e1d2c3b4a5f6e7    Invoice #12345                    billing@vendor.com    2025-01-09
16d1c2b3a4e5f6d7    Project update                    bob@example.com       2025-01-08
```

Message-level search (one row per email; add `--include-body` to fetch/decode bodies):

```bash
$ rata gmail messages search 'newer_than:7d' --max 3
ID                  THREAD             SUBJECT                           FROM                  DATE
18f1a2b3c4d5e6f7    9e8d7c6b5a4f3e2d    Meeting notes                     alice@example.com     2025-01-10
17e1d2c3b4a5f6e7    9e8d7c6b5a4f3e2d    Invoice #12345                    billing@vendor.com    2025-01-09
16d1c2b3a4e5f6d7    7f6e5d4c3b2a1908    Project update                    bob@example.com       2025-01-08
```

### JSON

Machine-readable output for scripting and automation:

```bash
$ rata gmail search 'newer_than:7d' --max 3 --json
{
  "threads": [
    {
      "id": "18f1a2b3c4d5e6f7",
      "snippet": "Meeting notes from today...",
      "messages": [...]
    },
    ...
  ]
}
```

```bash
$ rata gmail messages search 'newer_than:7d' --max 3 --json
{
  "messages": [
    {
      "id": "18f1a2b3c4d5e6f7",
      "threadId": "9e8d7c6b5a4f3e2d",
      "subject": "Meeting notes",
      "from": "alice@example.com",
      "date": "2025-01-10"
    },
    ...
  ]
}
```

```bash
$ rata gmail messages search 'newer_than:7d' --max 1 --include-body --json
{
  "messages": [
    {
      "id": "18f1a2b3c4d5e6f7",
      "threadId": "9e8d7c6b5a4f3e2d",
      "subject": "Meeting notes",
      "from": "alice@example.com",
      "date": "2025-01-10",
      "body": "Hi team — meeting notes..."
    }
  ]
}
```

Data goes to stdout, errors and progress to stderr for clean piping:

```bash
rata --json drive ls --max 5 | jq '.files[] | select(.mimeType=="application/pdf")'
```

Useful pattern:

- `rata --json ... | jq .`

Calendar JSON convenience fields:

- `startDayOfWeek` / `endDayOfWeek` on event payloads (derived from start/end).

## Examples

### Search recent emails and download attachments

```bash
# Search for emails from the last week
rata gmail search 'newer_than:7d has:attachment' --max 10

# Get thread details and download attachments
rata gmail thread get <threadId> --download
```

### Modify labels on a thread

```bash
# Archive and star a thread
rata gmail thread modify <threadId> --remove INBOX --add STARRED
```

### Create a calendar event with attendees

```bash
# Find a free time slot
rata calendar freebusy --calendars "primary" \
  --from 2025-01-15T00:00:00Z \
  --to 2025-01-16T00:00:00Z

# Create the meeting
rata calendar create primary \
  --summary "Team Standup" \
  --from 2025-01-15T10:00:00Z \
  --to 2025-01-15T10:30:00Z \
  --attendees "alice@example.com,bob@example.com"
```

### Find and download files from Drive

```bash
# Search for PDFs
rata drive search "invoice filetype:pdf" --max 20 --json | \
  jq -r '.files[] | .id' | \
  while read fileId; do
    rata drive download "$fileId"
  done
```

### Manage multiple accounts

```bash
# Check personal Gmail
rata gmail search 'is:unread' --account personal@gmail.com

# Check work Gmail
rata gmail search 'is:unread' --account work@company.com

# Or set default
export RATA_ACCOUNT=work@company.com
rata gmail search 'is:unread'
```

### Update a Google Sheet from a CSV

```bash
# Convert CSV to pipe-delimited format and update sheet
cat data.csv | tr ',' '|' | \
  rata sheets update <spreadsheetId> 'Sheet1!A1'
```

### Export Sheets / Docs / Slides

```bash
# Sheets
rata sheets export <spreadsheetId> --format pdf

# Docs
rata docs export <docId> --format docx

# Slides
rata slides export <presentationId> --format pptx
```

### Batch process Gmail threads

```bash
# Mark all emails from a sender as read
rata --json gmail search 'from:noreply@example.com' --max 200 | \
  jq -r '.threads[].id' | \
  xargs -n 50 rata gmail labels modify --remove UNREAD

# Archive old emails
rata --json gmail search 'older_than:1y' --max 200 | \
  jq -r '.threads[].id' | \
  xargs -n 50 rata gmail labels modify --remove INBOX

# Label important emails
rata --json gmail search 'from:boss@example.com' --max 200 | \
  jq -r '.threads[].id' | \
  xargs -n 50 rata gmail labels modify --add IMPORTANT
```

## Advanced Features

### Verbose Mode

Enable verbose logging for troubleshooting:

```bash
rata --verbose gmail search 'newer_than:7d'
# Shows API requests and responses
```

## Global Flags

All commands support these flags:

- `--account <email|alias|auto>` - Account to use (overrides RATA_ACCOUNT)
- `--enable-commands <csv>` - Allowlist top-level commands (e.g., `calendar,tasks`)
- `--json` - Output JSON to stdout (best for scripting)
- `--plain` - Output stable, parseable text to stdout (TSV; no colors)
- `--color <mode>` - Color mode: `auto`, `always`, or `never` (default: auto)
- `--force` - Skip confirmations for destructive commands
- `--no-input` - Never prompt; fail instead (useful for CI)
- `--verbose` - Enable verbose logging
- `--help` - Show help for any command

## Shell Completions

Generate shell completions for your preferred shell:

### Bash

```bash
# macOS (with Homebrew)
rata completion bash > $(brew --prefix)/etc/bash_completion.d/rata

# Linux
rata completion bash > /etc/bash_completion.d/rata

# Or load directly in your current session
source <(rata completion bash)
```

### Zsh

```zsh
# Generate completion file
rata completion zsh > "${fpath[1]}/_rata"

# Or add to .zshrc for automatic loading
echo 'eval "$(rata completion zsh)"' >> ~/.zshrc

# Enable completions if not already enabled
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

### Fish

```fish
rata completion fish > ~/.config/fish/completions/rata.fish
```

### PowerShell

```powershell
# Load for current session
rata completion powershell | Out-String | Invoke-Expression

# Or add to profile for all sessions
rata completion powershell >> $PROFILE
```

After installing completions, start a new shell session for changes to take effect.

## Development

After cloning, install tools:

```bash
make tools
```

Pinned tools (installed into `.tools/`):

- Format: `make fmt` (goimports + gofumpt)
- Lint: `make lint` (golangci-lint)
- Test: `make test`

CI runs format checks, tests, and lint on push/PR.

### Integration Tests (Live Google APIs)

Opt-in tests that hit real Google APIs using your stored `rata` credentials/tokens.

```bash
# Optional: override which account to use
export RATA_IT_ACCOUNT=you@gmail.com
export RATA_CLIENT=work
go test -tags=integration ./...
```

Tip: if you want to avoid macOS Keychain prompts during these runs, set `RATA_KEYRING_BACKEND=file` and `RATA_KEYRING_PASSWORD=...` (uses encrypted on-disk keyring).

### Live Test Script (CLI)

Fast end-to-end smoke checks against live APIs:

```bash
scripts/live-test.sh --fast
scripts/live-test.sh --account you@gmail.com --skip groups,keep,calendar-enterprise
scripts/live-test.sh --client work --account you@company.com
```

Script toggles:

- `--auth all,groups` to re-auth before running
- `--client <name>` to select OAuth client credentials
- `--strict` to fail on optional features (groups/keep/enterprise)
- `--allow-nontest` to override the test-account guardrail

Go test wrapper (opt-in):

```bash
RATA_LIVE=1 go test -tags=integration ./internal/integration -run Live
```

Optional env:
- `RATA_LIVE_FAST=1`
- `RATA_LIVE_SKIP=groups,keep`
- `RATA_LIVE_AUTH=all,groups`
- `RATA_LIVE_ALLOW_NONTEST=1`
- `RATA_LIVE_EMAIL_TEST=steipete+ratatest@gmail.com`
- `RATA_LIVE_GROUP_EMAIL=group@domain`
- `RATA_LIVE_CLASSROOM_COURSE=<courseId>`
- `RATA_LIVE_CLASSROOM_CREATE=1`
- `RATA_LIVE_CLASSROOM_ALLOW_STATE=1`
- `RATA_LIVE_TRACK=1`
- `RATA_LIVE_GMAIL_BATCH_DELETE=1`
- `RATA_LIVE_GMAIL_FILTERS=1`
- `RATA_LIVE_GMAIL_WATCH_TOPIC=projects/.../topics/...`
- `RATA_LIVE_CALENDAR_RESPOND=1`
- `RATA_LIVE_CALENDAR_RECURRENCE=1`
- `RATA_KEEP_SERVICE_ACCOUNT=/path/to/service-account.json`
- `RATA_KEEP_IMPERSONATE=user@workspace-domain`

### Make Shortcut

Build and run:

```bash
make rata auth add you@gmail.com
```

For clean stdout when scripting:

- Use `--` when the first arg is a flag: `make rata -- --json gmail search "from:me" | jq .`

## License

MIT

## Links

- [GitHub Repository](https://github.com/degree-analytics/ratatosk)
- [Gmail API Documentation](https://developers.google.com/gmail/api)
- [Google Calendar API Documentation](https://developers.google.com/calendar)
- [Google Drive API Documentation](https://developers.google.com/drive)
- [Google People API Documentation](https://developers.google.com/people)
- [Google Tasks API Documentation](https://developers.google.com/tasks)
- [Google Sheets API Documentation](https://developers.google.com/sheets)
- [Cloud Identity API Documentation](https://cloud.google.com/identity/docs/reference/rest)

## Credits

This project is inspired by Mario Zechner's original CLIs:

- [gmcli](https://github.com/badlogic/gmcli)
- [gccli](https://github.com/badlogic/gccli)
- [gdcli](https://github.com/badlogic/gdcli)
