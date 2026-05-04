# 🧭 gogcli — Google in your terminal.

![GitHub Repo Banner](https://ghrb.waren.build/banner?header=gogcli%F0%9F%A7%AD&subheader=Google+in+your+terminal&bg=f3f4f6&color=1f2937&support=true)
<!-- Created with GitHub Repo Banner by Waren Gonzaga: https://ghrb.waren.build -->

Fast, script-friendly CLI for Gmail, Calendar, Chat, Classroom, Drive, Docs, Slides, Sheets, Forms, Apps Script, Contacts, Tasks, People, Admin, Groups (Workspace), and Keep (Workspace-only). JSON-first output, multiple accounts, and flexible auth built in.

## Features

- **Gmail** - search threads/messages, send mail, view attachments, manage labels/drafts/filters/delegation/vacation settings, auto-reply once to matching mail, modify single messages, export filters, inspect history, and run Pub/Sub watch webhooks
- **Email tracking** - track opens for `gog gmail send --track` with a small Cloudflare Worker backend
- **Encrypted backups** - export Google account data to age-encrypted Git shards (`gog backup`)
- **Calendar** - list/create/update/delete events, manage invitations, aliases, subscriptions, team calendars, free/busy/conflicts, propose new times, focus/OOO/working-location events, recurrence, and reminders
- **Classroom** - manage courses, roster, coursework/materials, submissions, announcements, topics, invitations, guardians, profiles
- **Chat** - list/find/create spaces, list messages/threads, send messages and DMs, and manage emoji reactions (Workspace-only)
- **Drive** - list/search/upload/download files, scope search to folders or shared drives, replace uploads in-place, convert uploads (including Markdown to Google Doc), manage permissions/comments, organize folders, and list shared drives
- **Contacts** - search/create/update contacts, including addresses, relations, org/title metadata, custom fields, Workspace directory, and other contacts
- **Tasks** - manage tasklists and tasks: get/create/add/update/done/undo/delete/clear, plus repeat schedule materialization with RRULE aliases
- **Sheets** - read/write/update spreadsheets, insert rows/cols, manage tabs, named ranges, and Sheets tables, format/merge/freeze/resize cells, read/write notes, inspect formats, find/replace text, list links, and create/export sheets
- **Forms** - create/update forms, manage questions, inspect responses, and manage watches
- **Apps Script** - create/get/bind projects, inspect content, and run functions
- **Docs/Slides** - create/copy/export docs/slides, edit Docs by tab title or ID, import Markdown, do richer find-replace, export whole Docs or a single Docs tab, and generate Slides from Markdown or templates
- **People** - profile lookup and directory search helpers
- **Keep (Workspace only)** - list/get/search/create/delete notes and download attachments (service account + domain-wide delegation)
- **Admin (Workspace only)** - Workspace Admin users/groups commands for common directory operations
- **Groups** - list groups you belong to, view group members (Google Workspace)
- **Local time** - quick local/UTC time display for scripts and agents
- **Multiple accounts** - manage multiple Google accounts simultaneously, with account aliases and per-client OAuth buckets
- **Command allowlist + baked safety profiles** - restrict commands at runtime or build a fail-closed agent binary
- **Secure credential storage** using OS keyring or encrypted on-disk keyring (configurable)
- **Auto-refreshing tokens** - authenticate once, use indefinitely
- **Flexible auth** - OAuth refresh tokens, ADC, direct access tokens, service accounts, manual/remote flows, `--extra-scopes`, and proxy-safe callbacks
- **Least-privilege auth** - `--readonly`, `--drive-scope`, and `--gmail-scope` to request fewer scopes
- **Workspace service accounts** - domain-wide delegation auth (preferred when configured)
- **Parseable output** - JSON mode for scripting and automation (Calendar adds day-of-week fields)

## Installation

### Homebrew

```bash
brew install gogcli
```
### Arch User Repository

```bash
yay -S gogcli
```

### Windows

Download `gogcli_<version>_windows_amd64.zip` or `gogcli_<version>_windows_arm64.zip` from the GitHub release, extract `gog.exe`, and put that directory on `PATH`.

PowerShell example:

```powershell
$dir = "$env:LOCALAPPDATA\Programs\gogcli"
New-Item -ItemType Directory -Force $dir | Out-Null
# Extract gog.exe into $dir, then add $dir to your user PATH.
$userPath = [string][Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $dir) {
  [Environment]::SetEnvironmentVariable("Path", ($userPath.TrimEnd(";") + ";$dir").TrimStart(";"), "User")
}
$env:Path = "$env:Path;$dir"
gog --version
```

### Docker

Release images are published to GitHub Container Registry:

```bash
docker run --rm ghcr.io/steipete/gogcli:latest version
```

For authenticated automation in a container, mount a persistent config directory and use the encrypted file keyring:

```bash
docker volume create gogcli-config
docker run --rm -it \
  -e GOG_KEYRING_BACKEND=file \
  -e GOG_KEYRING_PASSWORD='change-me' \
  -v gogcli-config:/home/gog/.config/gogcli \
  ghcr.io/steipete/gogcli:latest auth add you@gmail.com --services gmail,calendar,drive
```

Subsequent runs can reuse the same volume and password:

```bash
docker run --rm \
  -e GOG_KEYRING_BACKEND=file \
  -e GOG_KEYRING_PASSWORD='change-me' \
  -v gogcli-config:/home/gog/.config/gogcli \
  ghcr.io/steipete/gogcli:latest gmail labels list --account you@gmail.com
```

### Build from Source

```bash
git clone https://github.com/steipete/gogcli.git
cd gogcli
make
```

Source builds require the Go version declared in `go.mod` (currently Go 1.26.2). Older distro packages such as Ubuntu 24.04's Go 1.22 are too old; install a current Go toolchain or use a release package.

Run:

```bash
./bin/gog --help
```

Help:

- `gog --help` shows top-level command groups.
- Drill down with `gog <group> --help` (and deeper subcommands).
- For the full expanded command list: `GOG_HELP=full gog --help`.
- Make shortcut: `make gog -- --help` (or `make gog -- gmail --help`).
- `make gog-help` shows CLI help (note: `make gog --help` is Make’s own help; use `--`).
- Version: `gog --version` or `gog version`.

## Quick Start

### 1. Get OAuth2 Credentials

Before adding an account, create OAuth2 credentials from Google Cloud Console:

1. Open the Google Cloud Console credentials page: https://console.cloud.google.com/apis/credentials
1. Create a project: https://console.cloud.google.com/projectcreate
2. Enable the APIs you need:
   - Admin SDK API: https://console.cloud.google.com/apis/api/admin.googleapis.com
   - Apps Script API: https://console.cloud.google.com/apis/api/script.googleapis.com
   - Cloud Identity API (Groups): https://console.cloud.google.com/apis/api/cloudidentity.googleapis.com
   - Gmail API: https://console.cloud.google.com/apis/api/gmail.googleapis.com
   - Google Calendar API: https://console.cloud.google.com/apis/api/calendar-json.googleapis.com
   - Google Chat API: https://console.cloud.google.com/apis/api/chat.googleapis.com
   - Google Docs API: https://console.cloud.google.com/apis/api/docs.googleapis.com
   - Google Drive API: https://console.cloud.google.com/apis/api/drive.googleapis.com
   - Google Classroom API: https://console.cloud.google.com/apis/api/classroom.googleapis.com
   - Google Keep API: https://console.cloud.google.com/apis/api/keep.googleapis.com
   - People API (Contacts): https://console.cloud.google.com/apis/api/people.googleapis.com
   - Google Tasks API: https://console.cloud.google.com/apis/api/tasks.googleapis.com
   - Google Sheets API: https://console.cloud.google.com/apis/api/sheets.googleapis.com
   - Google Forms API: https://console.cloud.google.com/apis/api/forms.googleapis.com
   - Google Slides API: https://console.cloud.google.com/apis/api/slides.googleapis.com
   If Google returns `accessNotConfigured` or says an API has not been used in the project, enable the API in the same Cloud project that owns your OAuth client JSON, then retry after the enablement propagates.

   `gog` does not require Google Workspace for normal user automation. For a
   consumer `gmail.com` account, a regular Google Cloud project plus a "Desktop
   app" OAuth client is enough for Gmail, Calendar, Drive, Docs, Sheets, Slides,
   Forms, Apps Script, Contacts/People, Tasks, and Classroom, as long as the
   matching APIs are enabled in that same Cloud project and the account grants
   the requested scopes.

   Google Workspace or Cloud Identity is only required for APIs Google restricts
   to managed domains, such as Chat, Cloud Identity Groups, Admin Directory, and
   Keep/domain-wide-delegation flows. If you authenticate a consumer Gmail
   account for those services, `gog` can store the scopes, but Google may still
   reject live API calls because the account is not a Workspace account.

   Apps Script has one extra user-side switch: after enabling
   `script.googleapis.com` in the Cloud project, open
   https://script.google.com/home/usersettings as the same Google account and
   turn on the Google Apps Script API. Without that per-user toggle, Apps Script
   calls can still return a 403 even when OAuth and the Cloud project API are
   configured correctly.
3. Configure OAuth consent screen: https://console.cloud.google.com/auth/branding
4. If your app is in "Testing", add test users: https://console.cloud.google.com/auth/audience
   - Testing-mode refresh tokens expire after 7 days for External apps that request Gmail/Drive/Calendar-style user-data scopes. For a personal consumer Gmail account, publish the OAuth app for long-lived refresh tokens; a small personal/unverified app can still show Google's unverified-app warning and user cap. Staying in Testing means re-authenticating every 7 days.
5. Create OAuth client:
   - Go to https://console.cloud.google.com/auth/clients
   - Click "Create Client"
   - Application type: "Desktop app"
   - Download the JSON file (usually named `client_secret_....apps.googleusercontent.com.json`)

### 2. Store Credentials

```bash
gog auth credentials ~/Downloads/client_secret_....json
```

For multiple OAuth clients/projects:

```bash
gog --client work auth credentials ~/Downloads/work-client.json
gog auth credentials list
```

### 3. Authorize Your Account

```bash
gog auth add you@gmail.com
```

This will open a browser window for OAuth authorization. The refresh token is stored securely in your system keychain.

Headless / remote server flows (no browser on the server):

Manual interactive flow (recommended):

```bash
gog auth add you@gmail.com --services user --manual
```

- The CLI prints an auth URL. Open it in a local browser.
- After approval, copy the full loopback redirect URL from the browser address bar.
- Paste that URL back into the terminal when prompted.
- If the pasted URL is accepted but the command appears stuck before success, OAuth likely completed and token storage is blocked by the OS keyring. Current builds time out keyring operations with a recovery hint; run `gog auth doctor`, or on headless Linux use `GOG_KEYRING_BACKEND=file` with `GOG_KEYRING_PASSWORD=...` and retry.

Split remote flow (`--remote`, useful for two-step/scripted handoff):

```bash
# Step 1: print auth URL (open it locally in a browser)
gog auth add you@gmail.com --services user --remote --step 1

# Step 2: paste the full redirect URL from your browser address bar
gog auth add you@gmail.com --services user --remote --step 2 --auth-url 'http://127.0.0.1:<port>/oauth2/callback?code=...&state=...'
```

- The `state` is cached on disk for a short time (about 10 minutes). If it expires, rerun step 1.
- Remote step 2 requires a redirect URL that includes `state` (state check mandatory).

Browser OAuth behind proxies / remote tunnels:

```bash
gog auth add you@gmail.com --listen-addr 0.0.0.0:8080 --redirect-host gog.example.com
gog auth manage --listen-addr 0.0.0.0:8080 --redirect-host gog.example.com
```

- `--listen-addr` changes where the local callback server binds.
- `--redirect-host` builds `https://<host>/oauth2/callback` for the OAuth redirect URI.
- The redirect URI must also be registered in your OAuth client settings.

Direct access token flow (headless/CI, no stored refresh token):

```bash
gog --access-token "$(gcloud auth print-access-token)" gmail labels list
```

- Also available as `GOG_ACCESS_TOKEN`
- Bypasses stored refresh tokens and keyring lookup
- Token expires in about 1 hour; no auto-refresh

### 4. Test Authentication

```bash
export GOG_ACCOUNT=you@gmail.com
gog gmail labels list
```

## Authentication & Secrets

### Accounts and tokens

`gog` stores your OAuth refresh tokens in a “keyring” backend. Default is `auto` (best available backend for your OS/environment).

Before you can run `gog auth add`, you must store OAuth client credentials once via `gog auth credentials <credentials.json>` (download a Desktop app OAuth client JSON from the Cloud Console). For multiple clients, use `gog --client <name> auth credentials ...`; tokens are isolated per client.

List accounts:

```bash
gog auth list
```

Verify tokens are usable (helps spot revoked/expired tokens):

```bash
gog auth list --check
```

If one file-keyring token cannot be decrypted, `gog auth list` still reports the readable accounts and prints the unreadable account with an error. Use `gog auth tokens list` to show token keys without decrypting them, then `gog auth tokens delete <email>` after confirming the affected account.

Diagnose keyring/password drift and refresh-token failures:

```bash
gog auth doctor
gog auth doctor --check
```

OAuth tokens store Google's stable OIDC subject (`sub`) alongside the account email. If Google renames an account email, re-authorizing the new address migrates matching subject-keyed tokens, aliases, client mappings, and defaults instead of orphaning the old email entry.

Accounts can be authorized either via OAuth refresh tokens or Workspace service accounts (domain-wide delegation). If a service account key is configured for an account, it takes precedence over OAuth refresh tokens (see `gog auth list`).

Show current auth state/services for the active account:

```bash
gog auth status
```

### Multiple OAuth clients

Use `--client` (or `GOG_CLIENT`) to select a named OAuth client:

```bash
gog --client work auth credentials ~/Downloads/work.json
gog --client work auth add you@company.com
```

Optional domain mapping for auto-selection:

```bash
gog --client work auth credentials ~/Downloads/work.json --domain example.com
```

How it works:

- Default client is `default` (stored in `credentials.json`).
- Named clients are stored as `credentials-<client>.json`.
- Tokens are isolated per client (`token:<client>:<email>`); defaults are per client too.

Client selection order (when `--client` is not set):

1) `--client` / `GOG_CLIENT`
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
gog auth credentials list
```

See `docs/auth-clients.md` for the full client selection and mapping rules.

### Keyring backend: Keychain vs encrypted file

Backends:

- `auto` (default): picks the best backend for the platform.
- `keychain`: macOS Keychain (recommended on macOS; avoids password management).
- `file`: encrypted on-disk keyring (requires a password).

On macOS, Keychain operations time out with a recovery hint if a permission prompt cannot surface; run the command from Terminal and choose "Always Allow", or switch to the file backend for headless runs.

Set backend via command (writes `keyring_backend` into `config.json`):

```bash
gog auth keyring file
gog auth keyring keychain
gog auth keyring auto
```

Show current backend + source (env/config/default) and config path:

```bash
gog auth keyring
```

Non-interactive runs (CI/ssh): file backend requires `GOG_KEYRING_PASSWORD`.
The file backend uses portable encoded filenames for stored keys, so account tokens work on Windows even when key names contain colons.
If you see `aes.KeyUnwrap(): integrity check failed`, first run `gog auth doctor`; the usual cause is that different shells/services/agents are using different `GOG_KEYRING_PASSWORD` values for the same encrypted token files.

```bash
export GOG_KEYRING_PASSWORD='...'
gog --no-input auth status
```

Force backend via env (overrides config):

```bash
export GOG_KEYRING_BACKEND=file
```

Precedence: `GOG_KEYRING_BACKEND` env var overrides `config.json`.

## Configuration

### Account Selection

Specify the account using either a flag or environment variable:

```bash
# Via flag
gog gmail search 'newer_than:7d' --account you@gmail.com

# Via alias
gog auth alias set work work@company.com
gog gmail search 'newer_than:7d' --account work

# Via environment
export GOG_ACCOUNT=you@gmail.com
gog gmail search 'newer_than:7d'

# Auto-select (default account or the single stored token)
gog gmail labels list --account auto
```

List configured accounts:

```bash
gog auth list
```

### Output

- Default: human-friendly tables on stdout.
- `--plain`: stable TSV on stdout (tabs preserved; best for piping to tools that expect `\t`).
- `--json`: JSON on stdout (best for scripting).
- Human-facing hints/progress go to stderr.
- Colors are enabled only in rich TTY output and are disabled automatically for `--json` and `--plain`.

### Service Scopes

By default, `gog auth add` requests access to the **user** services (see `gog auth services` for the current list and scopes).

To request fewer scopes:

```bash
gog auth add you@gmail.com --services drive,calendar
```

To request read-only scopes (write operations will fail with 403 insufficient scopes):

```bash
gog auth add you@gmail.com --services drive,calendar --readonly
```

To control Drive’s scope (default: `full`):

```bash
gog auth add you@gmail.com --services drive --drive-scope full
gog auth add you@gmail.com --services drive --drive-scope readonly
gog auth add you@gmail.com --services drive --drive-scope file
```

To control Gmail’s scope (default: `full`):

```bash
gog auth add you@gmail.com --services gmail --gmail-scope full
gog auth add you@gmail.com --services gmail --gmail-scope readonly
# Example: readonly on both Gmail and Drive
gog auth add you@gmail.com --services gmail,drive --gmail-scope readonly --drive-scope readonly
# Example: append one custom scope beyond the built-in Gmail scope set
gog auth add you@gmail.com --services gmail --extra-scopes https://www.googleapis.com/auth/gmail.labels
```

Notes:

- `--drive-scope readonly` is enough for listing/downloading/exporting via Drive (write operations will 403).
- `--drive-scope file` is write-capable (limited to files created/opened by this app) and can’t be combined with `--readonly`.
- `--gmail-scope readonly` requests `gmail.readonly` (no modify/settings write scopes).
- `--extra-scopes` appends additional OAuth scope URIs after the built-in service scope set; remote step-1 guidance replays it so step 2 requests the same token.
- For `--readonly`, `--drive-scope readonly|file`, or `--gmail-scope readonly`, auth disables Google `include_granted_scopes` to prevent old broader grants from silently accumulating.

If you need to add services later and Google doesn't return a refresh token, re-run with `--force-consent`:

```bash
gog auth add you@gmail.com --services user --force-consent
# Or add just Sheets
gog auth add you@gmail.com --services sheets --force-consent
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
| slides | yes | Slides API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/presentations` | Create/edit presentations |
| contacts | yes | People API | `https://www.googleapis.com/auth/contacts`<br>`https://www.googleapis.com/auth/contacts.other.readonly`<br>`https://www.googleapis.com/auth/directory.readonly` | Contacts + other contacts + directory |
| tasks | yes | Tasks API | `https://www.googleapis.com/auth/tasks` |  |
| sheets | yes | Sheets API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/spreadsheets` | Export via Drive |
| people | yes | People API | `profile` | OIDC profile scope |
| forms | yes | Forms API | `https://www.googleapis.com/auth/forms.body`<br>`https://www.googleapis.com/auth/forms.responses.readonly` |  |
| appscript | yes | Apps Script API | `https://www.googleapis.com/auth/script.projects`<br>`https://www.googleapis.com/auth/script.deployments`<br>`https://www.googleapis.com/auth/script.processes` |  |
| ads | yes | Google Ads API | `https://www.googleapis.com/auth/adwords` | OAuth scope only |
| groups | no | Cloud Identity API | `https://www.googleapis.com/auth/cloud-identity.groups.readonly` | Workspace only |
| keep | no | Keep API | `https://www.googleapis.com/auth/keep` | Workspace only; service account (domain-wide delegation) |
| admin | no | Admin SDK Directory API | `https://www.googleapis.com/auth/admin.directory.user`<br>`https://www.googleapis.com/auth/admin.directory.group`<br>`https://www.googleapis.com/auth/admin.directory.group.member` | Workspace only; service account with domain-wide delegation required |
<!-- auth-services:end -->

### Service Accounts (Workspace only)

A service account is a non-human Google identity that belongs to a Google Cloud project. In Google Workspace, a service account can impersonate a user via **domain-wide delegation** (admin-controlled) and access APIs like Gmail/Calendar/Drive as that user.

In `gog`, service accounts are an **optional auth method** that can be configured per account email. If a service account key is configured for an account, it takes precedence over OAuth refresh tokens (see `gog auth list`).

#### 1) Create a Service Account (Google Cloud)

1. Create (or pick) a Google Cloud project.
2. Enable the APIs you’ll use (e.g. Gmail, Calendar, Drive, Sheets, Docs, People, Tasks, Cloud Identity).
3. Go to **IAM & Admin → Service Accounts** and create a service account.
4. In the service account details, enable **Domain-wide delegation**.
5. Create a key (**Keys → Add key → Create new key → JSON**) and download the JSON key file.

#### 2) Allowlist scopes (Google Workspace Admin Console)

Domain-wide delegation is enforced by Workspace admin settings.

1. Open **Admin console → Security → API controls → Domain-wide delegation**.
2. Add a new API client:
   - Client ID: use the service account’s “Client ID” from Google Cloud.
   - OAuth scopes: comma-separated list of scopes you want to allow (copy from `gog auth services` and/or your `gog auth add --services ...` usage).

If a scope is missing from the allowlist, service-account token minting can fail (or API calls will 403 with insufficient permissions).

#### 3) Configure `gog` to use the service account

Store the key for the user you want to impersonate:

```bash
gog auth service-account set you@yourdomain.com --key ~/Downloads/service-account.json
```

Verify `gog` is preferring the service account for that account:

```bash
gog --account you@yourdomain.com auth status
gog auth list
```

### Google Keep (Workspace only)

Keep requires Workspace + domain-wide delegation. You can configure it via the generic service-account command above (recommended), or the legacy Keep helper:

```bash
gog auth service-account set you@yourdomain.com --key ~/Downloads/service-account.json
gog keep list --account you@yourdomain.com
gog keep get <noteId> --account you@yourdomain.com
gog keep create --title "Todo" --item "Milk" --item "Eggs" --account you@yourdomain.com
gog keep delete <noteId> --account you@yourdomain.com --force
```

### Environment Variables

- `GOG_ACCOUNT` - Default account email or alias to use (avoids repeating `--account`; otherwise uses keyring default or a single stored token)
- `GOG_ACCESS_TOKEN` - Use a provided access token directly (headless/CI; no auto-refresh)
- `GOG_CLIENT` - OAuth client name (selects stored credentials + token bucket)
- `GOG_JSON` - Default JSON output
- `GOG_PLAIN` - Default plain output
- `GOG_COLOR` - Color mode: `auto` (default), `always`, or `never`
- `GOG_TIMEZONE` - Default output timezone for Calendar/Gmail (IANA name, `UTC`, or `local`)
- `GOG_ENABLE_COMMANDS` - Comma-separated allowlist of commands; dot paths allowed (e.g., `calendar,tasks,gmail.search`)
- `GOG_DISABLE_COMMANDS` - Comma-separated denylist of commands; dot paths allowed (e.g., `gmail.send,gmail.drafts.send`)
- `GOG_GMAIL_NO_SEND` - Block Gmail send operations
- `GOG_KEYRING_SERVICE_NAME` - Override the keyring namespace/service name (default: `gogcli`)

### Config File (JSON5)

Find the actual config path in `gog --help` or `gog auth keyring`.

Typical paths:

- macOS: `~/Library/Application Support/gogcli/config.json`
- Linux: `~/.config/gogcli/config.json` (or `$XDG_CONFIG_HOME/gogcli/config.json`)
- Windows: `%AppData%\\gogcli\\config.json`

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
  // Optional safety guard: block Gmail send operations
  gmail_no_send: true,
  no_send_accounts: {
    "agent@example.com": true,
  },
}
```

### Config Commands

```bash
gog config path
gog config list
gog config keys
gog config get timezone
gog config set timezone UTC
gog config unset timezone
```

### Account Aliases

```bash
gog auth alias set work work@company.com
gog auth alias list
gog auth alias unset work
```

Aliases work anywhere you pass `--account` or `GOG_ACCOUNT` (reserved: `auto`, `default`).

### Command Guards (Sandboxing)

```bash
# Only allow calendar + tasks commands for an agent
gog --enable-commands calendar,tasks calendar events --today

# Allow one Gmail read path, but block Gmail writes
gog --enable-commands gmail.search --disable-commands gmail.send gmail search from:me

# Same via env
export GOG_ENABLE_COMMANDS=calendar,tasks
export GOG_DISABLE_COMMANDS=gmail.send,gmail.drafts.send
gog tasks list <tasklistId>

# Extra Gmail send guard
gog --gmail-no-send gmail send --to someone@example.com --subject Test --body Test
gog config no-send set agent@example.com
```

For stronger isolation, build a dedicated binary with an embedded safety profile:

```bash
./build-safe.sh safety-profiles/agent-safe.yaml -o bin/gog-agent-safe
./build-safe.sh safety-profiles/readonly.yaml -o bin/gog-readonly
```

Baked profiles are checked after CLI parsing and before any command runs. They are
fail-closed and cannot be changed by config, environment variables, or runtime
allowlist flags. See `docs/safety-profiles.md`.
 
## Security

### Credential Storage

OAuth credentials are stored securely in your system's keychain:
- **macOS**: Keychain Access
- **Linux**: Secret Service (GNOME Keyring, KWallet)
- **Windows**: Credential Manager

The CLI uses [github.com/99designs/keyring](https://github.com/99designs/keyring) for secure storage.

If no OS keychain backend is available (e.g., Linux/WSL/container), keyring can fall back to an encrypted on-disk store and may prompt for a password; for non-interactive runs set `GOG_KEYRING_PASSWORD`.

### Keychain Prompts (macOS)

macOS Keychain may prompt more than you’d expect when the “app identity” keeps changing (different binary path, `go run` temp builds, rebuilding to new `./bin/gog`, multiple copies). Keychain treats those as different apps, so it asks again.

Options:

- **Default (recommended):** keep using Keychain (secure) and run a stable `gog` binary path to reduce repeat prompts.
- **Force Keychain:** `GOG_KEYRING_BACKEND=keychain` (disables any file-backend fallback).
- **Avoid Keychain prompts entirely:** `GOG_KEYRING_BACKEND=file` (stores encrypted entries on disk under your config dir).
  - To avoid password prompts too (CI/non-interactive): set `GOG_KEYRING_PASSWORD=...` (tradeoff: secret in env).
- **Use a separate keyring namespace:** `GOG_KEYRING_SERVICE_NAME=custom-gog` (default: `gogcli`).

### Best Practices

- **Never commit OAuth client credentials** to version control
- Store client credentials outside your project directory
- Use different OAuth clients for development and production
- Re-authorize with `--force-consent` if you suspect token compromise
- Remove unused accounts with `gog auth remove <email>`

### OAuth Client IDs in Open Source

Some open source Google CLIs ship a pre-configured OAuth client ID/secret copied from other desktop apps to avoid OAuth consent verification, testing-user limits, or quota issues. This makes the consent screen/security emails show the other app’s name and can stop working at any time.

`gogcli` does not do this. Supported auth:

- Your own OAuth Desktop client JSON via `gog auth credentials ...` + `gog auth add ...`
- Google Workspace service accounts with domain-wide delegation (Workspace only)

For consumer Gmail accounts, there is no `gogcli` workaround for Google's OAuth publishing status. If the OAuth app is External + Testing and requests Gmail or other user-data scopes, Google expires the refresh token after 7 days. To avoid weekly re-auth, move the OAuth app to production/published status; for personal use under Google's unverified-app cap, this can still work without shipping a public app. Workspace Internal apps and service-account delegation only help Workspace-owned accounts, not `@gmail.com` mailboxes.

## Commands

Flag aliases:
- `--out` also accepts `--output`.
- `--out-dir` also accepts `--output-dir` (Gmail thread attachment downloads).
- Drive download/export commands accept `--out -` to write file bytes to stdout; do not combine this with `--json`.

### Authentication

```bash
gog auth credentials <path>           # Store OAuth client credentials
gog auth credentials list             # List stored OAuth client credentials
gog auth credentials remove work      # Remove one OAuth client plus its tokens/domain mappings
gog auth credentials remove all       # Remove all stored OAuth clients plus their tokens/domain mappings
gog --client work auth credentials <path>  # Store named OAuth client credentials
gog auth add <email>                  # Authorize and store refresh token
gog auth add <email> --services gmail --gmail-scope readonly  # Gmail read-only token
gog auth add <email> --listen-addr 0.0.0.0:8080 --redirect-host gog.example.com
gog auth service-account set <email> --key <path>  # Configure service account impersonation (Workspace only)
gog auth service-account status <email>            # Show service account status
gog auth service-account unset <email>             # Remove service account
gog auth keep <email> --key <path>                 # Legacy alias (Keep)
gog auth keyring [backend]            # Show/set keyring backend (auto|keychain|file)
gog auth status                       # Show current auth state/services
gog auth services                     # List available services and OAuth scopes
gog auth list                         # List stored accounts
gog auth list --check                 # Validate stored refresh tokens
gog auth remove <email>               # Remove a stored refresh token
gog auth manage                       # Open accounts manager in browser
gog auth manage --listen-addr 0.0.0.0:8080 --redirect-host gog.example.com
gog auth tokens                       # Manage stored refresh tokens
```

### Keep (Workspace only)

```bash
gog keep list --account you@yourdomain.com
gog keep get <noteId> --account you@yourdomain.com
gog keep search <query> --account you@yourdomain.com
gog keep create --title "Todo" --item "Milk" --item "Eggs" --account you@yourdomain.com
gog keep create --title "Note" --text "Remember this" --account you@yourdomain.com
gog keep delete <noteId> --account you@yourdomain.com --force
gog keep attachment <attachmentName> --account you@yourdomain.com --out ./attachment.bin
```

### Gmail

```bash
# Search and read
gog gmail search 'newer_than:7d' --max 10
gog gmail thread get <threadId>
gog gmail thread get <threadId> --download              # Download attachments to current dir
gog gmail thread get <threadId> --download --out-dir ./attachments
gog gmail thread get <threadId> --sanitize-content      # Agent-oriented sanitized content output
gog gmail get <messageId>
gog gmail get <messageId> --format metadata
gog gmail get <messageId> --sanitize-content            # Agent-oriented sanitized content output
gog gmail attachment <messageId> <attachmentId>
gog gmail attachment <messageId> <attachmentId> --out ./attachment.bin
gog gmail url <threadId>              # Print Gmail web URL
gog gmail thread modify <threadId> --add STARRED --remove INBOX

# Send and compose
gog gmail send --to a@b.com --subject "Hi" --body "Plain fallback"
gog gmail send --to a@b.com --subject "Hi" --body-file ./message.txt
gog gmail send --to a@b.com --subject "Hi" --body-file -   # Read body from stdin
gog gmail send --to a@b.com --subject "Hi" --body "Plain fallback" --body-html "<p>Hello</p>"
gog gmail send --to a@b.com --subject "Hi" --body "Hello" --signature
gog gmail send --to a@b.com --subject "Hi" --body "Hello" --from alias@example.com --signature
gog gmail send --to a@b.com --subject "Hi" --body "Hello" --signature-from alias@example.com
gog gmail send --to a@b.com --subject "Hi" --body "Hello" --signature-file ./signature.html
gog gmail forward <messageId> --to a@b.com --note "FYI"
gog gmail forward <messageId> --to a@b.com --skip-attachments
# Reply + include quoted original message (auto-generates HTML quote unless you pass --body-html)
gog gmail send --reply-to-message-id <messageId> --quote --to a@b.com --subject "Re: Hi" --body "My reply"
# Draft reply + quote (create requires explicit reply target)
gog gmail drafts create --reply-to-message-id <messageId> --quote --subject "Re: Hi" --body "My reply"
# Draft reply + quote (update accepts explicit target; else falls back to latest non-draft, non-self message in thread)
gog gmail drafts update <draftId> --reply-to-message-id <messageId> --quote --subject "Re: Hi" --body "My reply"
gog gmail drafts update <draftId> --quote --subject "Re: Hi" --body "My reply"
gog gmail drafts list
gog gmail drafts create --subject "Draft" --body "Body"
gog gmail drafts create --to a@b.com --subject "Draft" --body "Body"
gog gmail drafts update <draftId> --subject "Draft" --body "Body"
gog gmail drafts update <draftId> --to a@b.com --subject "Draft" --body "Body"
gog gmail drafts send <draftId>
gog gmail autoreply 'from:alerts@example.com newer_than:7d' --body-file ./reply.txt --label AutoReplied --dry-run

# Labels
gog gmail labels list
gog gmail labels get INBOX --json  # Includes message counts
gog gmail labels create "My Label"
gog gmail labels create "Projects/Review"  # Nested Gmail label
# Gmail rejects slash/hyphen collisions such as Projects/Review when Projects-Review exists.
gog gmail labels rename "Old Label" "New Label"
gog gmail labels style "My Label" --text-color "#ffffff" --background-color "#4285f4"
gog gmail labels modify <threadId> --add STARRED --remove INBOX
gog gmail labels delete <labelIdOrName>  # Deletes user label (guards system labels; confirm)

# Batch operations
gog gmail trash <messageId> <messageId>          # Move to trash; works with gmail.modify
gog gmail trash --query 'older_than:30d' --max 100
gog gmail batch delete <messageId> <messageId>   # Permanent delete; requires https://mail.google.com/
gog gmail batch modify <messageId> <messageId> --add STARRED --remove INBOX

# Filters
gog gmail filters list
gog gmail filters create --from 'noreply@example.com' --add-label 'Notifications'
gog gmail filters delete <filterId>
gog gmail filters export --out ./filters.json

# Settings
gog gmail autoforward get
gog gmail autoforward enable --email forward@example.com
gog gmail autoforward disable
gog gmail forwarding list
gog gmail forwarding add --email forward@example.com
gog gmail sendas list
gog gmail sendas create --email alias@example.com
gog gmail vacation get
gog gmail vacation enable --subject "Out of office" --message "..."
gog gmail vacation disable

# Delegation (G Suite/Workspace)
gog gmail delegates list
gog gmail delegates add --email delegate@example.com
gog gmail delegates remove --email delegate@example.com

# Watch (Pub/Sub push)
gog gmail watch start --topic projects/<p>/topics/<t> --label INBOX
gog gmail watch serve --bind 127.0.0.1 --token <shared> --hook-url http://127.0.0.1:18789/hooks/agent
gog gmail watch serve --bind 0.0.0.0 --verify-oidc --oidc-email <svc@...> --hook-url <url>
gog gmail watch serve --bind 127.0.0.1 --token <shared> --fetch-delay 5 --hook-url http://127.0.0.1:18789/hooks/agent
gog gmail watch serve --bind 127.0.0.1 --token <shared> --exclude-labels SPAM,TRASH --hook-url http://127.0.0.1:18789/hooks/agent
gog gmail history --since <historyId>
```

Gmail watch (Pub/Sub push):
- Create Pub/Sub topic + push subscription (OIDC preferred; shared token ok for dev).
- Full flow + payload details: `docs/watch.md`.
- `watch serve --fetch-delay` defaults to `3s` and helps avoid Gmail History indexing races after push delivery.
- `watch serve --exclude-labels` defaults to `SPAM,TRASH`; IDs are case-sensitive.

Sanitized Gmail content (`--sanitize-content`, alias `--safe`):
- Converts HTML bodies to text with an HTML parser and removes script/style content.
- Replaces HTTP(S) URLs with `[url removed]` after decoding HTML entities.
- Omits raw Gmail `payload`/RFC822 data and unsubscribe links from sanitized JSON envelopes.
- Rejects `gmail get --format raw`, because raw output cannot be sanitized.

This reduces prompt-injection, phishing-link, and tracking-link exposure for agents.
It is not a sandbox; use command guards or baked safety profiles for command
boundaries.

### Encrypted Backup

```bash
gog backup init --repo ~/Projects/backup-gog --remote https://github.com/steipete/backup-gog.git
gog backup push --services all --account you@gmail.com
gog backup status
gog backup verify
gog backup cat data/gmail/<account-hash>/labels.jsonl.gz.age --pretty
gog backup export --out ~/Documents/gog-backup-export
gog backup export --no-pull --out ~/Library/CloudStorage/Dropbox/backup/gog --gmail-format markdown
```

For a bounded first run:

```bash
gog backup push --services gmail --account you@gmail.com --query 'newer_than:7d' --max 25
```

Backups use age-encrypted JSONL gzip shards under `data/` and completed Gmail
checkpoint shards under `checkpoints/`. `gog` stores the private age identity
locally at `~/.gog/age.key`; GitHub only receives public `age1...` recipients,
`manifest.json`, and encrypted `*.jsonl.gz.age` payloads.
The private `AGE-SECRET-KEY-...` value must stay local or in a password manager.

Supported backup services are `gmail`, `gmail-settings`, `calendar`,
`contacts`, `tasks`, `drive`, `workspace`, `appscript`, `chat`, `classroom`,
`groups`, `admin`, and `keep`; `all` expands to those services. Drive stores
metadata, permissions, comments, revisions, and exported Google-native file
content by default. Non-Google binary Drive files are metadata-only unless
`--drive-binary-contents` is set. `--drive-content-timeout` turns a stuck
per-file export into an encrypted error row instead of wedging the run. Gmail
raw-message fetches and message-list pages use a local cache by default so
interrupted full-mailbox backups can resume. Full Gmail runs build encrypted
message shards from cached messages instead of keeping the whole mailbox in
memory; progress is written to stderr while stdout stays parseable. Cached
Gmail runs also push incomplete encrypted checkpoint commits during long fetches
by default (`--gmail-checkpoint-rows`, `--gmail-checkpoint-interval`,
`--no-gmail-checkpoints`). Checkpoint files are split by row count and a
conservative plaintext byte ceiling to avoid GitHub blob rejections. Checkpoint
commits push through a single ordered background queue so cached Gmail fetching
can continue while GitHub uploads run; the final completed backup waits for the
queue to drain before updating the authoritative manifest. When a cached Gmail
run completes, the final manifest promotes the completed checkpoint message
shards instead of re-encrypting the mailbox into a second giant Git push.
Checkpoints live under `checkpoints/` and do not become authoritative until the
final `manifest.json` references them. Use `--gmail-refresh-cache` to force a
refetch.
Workspace inventories
Docs/Sheets/Slides and backs up Forms/responses discovered through Drive; add
`--workspace-native` for full native Docs/Sheets/Slides API JSON.
Optional Workspace-only services use `--best-effort` by default, recording
permission/auth errors as encrypted error shards instead of stopping the run.

Use `gog backup cat` to decrypt one shard as JSONL, or `gog backup export` to
write a local plaintext copy. By default Gmail messages export as `.eml` files.
Use `--gmail-format markdown` for a readable mirror with `message.md` files and
extracted `attachments/` folders, or `--gmail-format both` to keep Markdown and
`.eml` side by side. `--gmail-attachments none` keeps Markdown notes without
writing attachment files. Drive contents export as normal files under
`drive/<account-hash>/files/` with an `index.jsonl`; other services export as
verified JSONL under `raw/`. That export is intentionally unencrypted; keep it
out of Git, shared folders, and cloud sync unless that is intentional.
Use `--no-pull` when exporting from a local backup repository that another
process is already updating.

`manifest.json` is intentionally cleartext for cheap status and verification.
It exposes metadata: export time, service names, account hashes, shard paths,
row counts, encrypted byte sizes, plaintext verification hashes, backup cadence,
and which shards changed. It does not contain email bodies, subjects, senders,
recipients, raw MIME, labels, Drive filenames, contacts, or event titles.

Security boundary: GitHub cannot read Google content without the age identity.
Repository writers can still replace backup contents with different encrypted
data, so keep write access tight and review unexpected backup commits. If the
age identity leaks, rotate recipients and re-encrypt; old Git history may still
contain shards decryptable with the leaked key. See `docs/backup.md`.

### Email Tracking

Track when recipients open your emails:

```bash
# Set up local tracking config (per-account; generates keys; follow printed deploy steps)
gog gmail track setup --worker-url https://gog-email-tracker.<acct>.workers.dev

# Send with tracking
gog gmail send --to recipient@example.com --subject "Hello" --body-html "<p>Hi!</p>" --track

# Check opens
gog gmail track opens <tracking_id>
gog gmail track opens --to recipient@example.com

# View status
gog gmail track status
```

Docs: `docs/email-tracking.md` (setup/deploy) + `docs/email-tracking-worker.md` (internals).

**Notes:** `--track` requires exactly 1 recipient (no cc/bcc) and an HTML body (`--body-html` or `--quote`). Use `--track-split` to send per-recipient messages with individual tracking ids. The tracking worker stores IP/user-agent + coarse geo by default.

### Calendar

```bash
# Calendars
gog calendar calendars
gog calendar create-calendar "Team Calendar" --timezone Europe/London
gog calendar acl <calendarId>         # List access control rules
gog calendar colors                   # List available event/calendar colors
gog calendar time --timezone America/New_York
gog calendar users                    # List workspace users (use email as calendar ID)

# Events (with timezone-aware time flags)
gog calendar events <calendarId> --today                    # Today's events
gog calendar events <calendarId> --tomorrow                 # Tomorrow's events
gog calendar events <calendarId> --week                     # This week (Mon-Sun by default; use --week-start)
gog calendar events <calendarId> --days 3                   # Next 3 days
gog calendar events <calendarId> --from today --to friday   # Relative dates
gog calendar events <calendarId> --from today --to friday --weekday   # Include weekday columns
gog calendar events <calendarId> --from 2025-01-01T00:00:00Z --to 2025-01-08T00:00:00Z
gog calendar events --all             # Fetch events from all calendars
gog calendar events --calendars 1,3   # Fetch events from calendar indices (see gog calendar calendars)
gog calendar events --cal Work --cal Personal  # Fetch events from calendars by name/ID
gog calendar event <calendarId> <eventId>
gog calendar get <calendarId> <eventId>                     # Alias for event
gog calendar search "meeting" --today
gog calendar search "meeting" --tomorrow
gog calendar search "meeting" --days 365
gog calendar search "meeting" --from 2025-01-01T00:00:00Z --to 2025-01-31T00:00:00Z --max 50

# Search defaults to 30 days ago through 90 days ahead unless you set --from/--to/--today/--week/--days.
# Tip: set GOG_CALENDAR_WEEKDAY=1 to default --weekday for calendar events output.

# JSON event output includes timezone and localized times (useful for agents).
gog calendar get <calendarId> <eventId> --json
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
gog calendar team <group-email> --today           # Show team's events for today
gog calendar team <group-email> --week            # Show team's events for the week (use --week-start)
gog calendar team <group-email> --freebusy        # Show only busy/free blocks (faster)
gog calendar team <group-email> --query "standup" # Filter by event title

# Create and update
gog calendar create <calendarId> \
  --summary "Meeting" \
  --from 2025-01-15T10:00:00Z \
  --to 2025-01-15T11:00:00Z

gog calendar create <calendarId> \
  --summary "Team Sync" \
  --from 2025-01-15T14:00:00Z \
  --to 2025-01-15T15:00:00Z \
  --attendees "alice@example.com,bob@example.com" \
  --location "Zoom"

gog calendar create <calendarId> \
  --summary "Flight" \
  --from 2026-08-13T13:40:00+02:00 --start-timezone Europe/Rome \
  --to 2026-08-13T17:00:00-04:00 --end-timezone America/New_York

gog calendar update <calendarId> <eventId> \
  --summary "Updated Meeting" \
  --from 2025-01-15T11:00:00Z \
  --to 2025-01-15T12:00:00Z

# Send notifications when creating/updating
gog calendar create <calendarId> \
  --summary "Team Sync" \
  --from 2025-01-15T14:00:00Z \
  --to 2025-01-15T15:00:00Z \
  --send-updates all

gog calendar update <calendarId> <eventId> \
  --send-updates externalOnly

# Move an event to another calendar, changing the event organizer.
gog calendar move <calendarId> <eventId> <destinationCalendarId> \
  --send-updates all

# Default: no attendee notifications unless you pass --send-updates.
gog calendar delete <calendarId> <eventId> \
  --send-updates all --force

# Recurrence + reminders
gog calendar create <calendarId> \
  --summary "Payment" \
  --from 2025-02-11T09:00:00-03:00 \
  --to 2025-02-11T09:15:00-03:00 \
  --rrule "RRULE:FREQ=MONTHLY;BYMONTHDAY=11" \
  --reminder "email:3d" \
  --reminder "popup:30m"

# Special event types via --event-type (focus-time/out-of-office/working-location)
gog calendar create primary \
  --event-type focus-time \
  --from 2025-01-15T13:00:00Z \
  --to 2025-01-15T14:00:00Z

gog calendar create primary \
  --event-type out-of-office \
  --from 2025-01-20 \
  --to 2025-01-21 \
  --all-day

gog calendar create primary \
  --event-type working-location \
  --working-location-type office \
  --working-office-label "HQ" \
  --from 2025-01-22 \
  --to 2025-01-23

# Dedicated shortcuts (same event types, more opinionated defaults)
gog calendar focus-time --from 2025-01-15T13:00:00Z --to 2025-01-15T14:00:00Z
gog calendar out-of-office --from 2025-01-20 --to 2025-01-21 --all-day
gog calendar working-location --type office --office-label "HQ" --from 2025-01-22 --to 2025-01-23
# Add attendees without replacing existing attendees/RSVP state
gog calendar update <calendarId> <eventId> \
  --add-attendee "alice@example.com,bob@example.com"

gog calendar delete <calendarId> <eventId>

# Invitations
gog calendar respond <calendarId> <eventId> --status accepted
gog calendar respond <calendarId> <eventId> --status declined
gog calendar respond <calendarId> <eventId> --status tentative
gog calendar respond <calendarId> <eventId> --status declined --send-updates externalOnly

# Propose a new time (browser-only flow; API limitation)
gog calendar propose-time <calendarId> <eventId>
gog calendar propose-time <calendarId> <eventId> --open
gog calendar propose-time <calendarId> <eventId> --decline --comment "Can we do 5pm?"

# Availability
gog calendar freebusy --calendars "primary,work@example.com" \
  --from 2025-01-15T00:00:00Z \
  --to 2025-01-16T00:00:00Z
gog calendar freebusy --cal Work --from 2025-01-15T00:00:00Z --to 2025-01-16T00:00:00Z

gog calendar conflicts --calendars "primary,work@example.com" \
  --today                             # Today's conflicts
gog calendar conflicts --all --today # Check conflicts across all calendars
```

### Time

```bash
gog time now
gog time now --timezone UTC
```

### Drive

When you turn a Markdown file into a Google Doc, use **`--convert`** (extension-based) or **`--convert-to doc`**. Leading YAML frontmatter between **`---`** lines is **removed before upload** unless you pass **`--keep-frontmatter`**. That step only looks for opening and closing delimiter lines—it is **not** a full YAML parse, so odd edge cases may need **`--keep-frontmatter`** or editing the file first.

```bash
# List and search
gog drive ls --max 20
gog drive ls --parent <folderId> --max 20
gog drive ls --all --max 20               # List across all accessible files (cannot combine with --parent)
gog drive ls --no-all-drives            # Only list from "My Drive"
gog drive search "invoice" --max 20
gog drive search "invoice" --no-all-drives
gog drive search "invoice" --parent <folderId>       # Direct children of one folder
gog drive search "invoice" --drive <sharedDriveId>   # Search within one shared drive
gog drive search "mimeType = 'application/pdf'" --raw-query
gog drive get <fileId>                # Get file metadata
gog drive url <fileId>                # Print Drive web URL
gog drive copy <fileId> "Copy Name"

# Upload and download
gog drive upload ./path/to/file --parent <folderId>
gog drive upload ./path/to/file --replace <fileId>  # Replace file content in-place (preserves shared link)
gog drive upload ./report.docx --convert
gog drive upload ./chart.png --convert-to sheet
gog drive upload ./report.docx --convert --name report.docx
gog drive upload ./notes.md --convert                              # Markdown → Google Doc (or use --convert-to doc)

# Large non-JSON uploads print progress on stderr.
gog drive download <fileId> --out ./downloaded.bin
gog drive download <fileId> --format pdf --out ./exported.pdf     # Google Workspace files only
gog drive download <fileId> --format docx --out ./doc.docx
gog drive download <fileId> --format md --out ./note.md            # Google Doc → Markdown
gog drive download <fileId> --tab "Notes" --format pdf --out ./notes.pdf
gog drive download <fileId> --format pptx --out ./slides.pptx
gog drive download <fileId> --out - > downloaded.bin

# Organize
gog drive mkdir "New Folder"
gog drive mkdir "New Folder" --parent <parentFolderId>
gog drive rename <fileId> "New Name"
gog drive move <fileId> --parent <destinationFolderId>
gog drive delete <fileId>             # Move to trash
gog drive delete <fileId> --permanent # Permanently delete

# Permissions
gog drive permissions <fileId>
gog drive share <fileId> --to user --email user@example.com --role reader
gog drive share <fileId> --to user --email user@example.com --role writer
gog drive share <fileId> --to user --email reviewer@example.com --role commenter
gog drive share <fileId> --to domain --domain example.com --role reader
gog drive unshare <fileId> --permission-id <permissionId>

# Shared drives (Team Drives)
gog drive drives --max 100

# Request extra fields from the Drive API (closes #486)
gog drive ls --fields "files(id,name,thumbnailLink),nextPageToken"
gog drive get <fileId> --fields "id,name,thumbnailLink,imageMediaMetadata"

# Raw API dump (lossless JSON for scripting/LLMs)
gog drive raw <fileId>                            # fields=*, sensitive fields redacted by default
gog drive raw <fileId> --fields "id,name,thumbnailLink"  # honors user-named fields verbatim
gog drive raw <fileId> --pretty
```

### Docs / Slides / Sheets

```bash
# Docs
gog docs info <docId>
gog docs cat <docId> --max-bytes 10000
gog docs create "My Doc"
gog docs create "My Doc" --file ./doc.md            # Import markdown
gog docs create "My Doc" --pageless
gog docs copy <docId> "My Doc Copy"
gog docs export <docId> --format pdf --out ./doc.pdf
gog docs export <docId> --tab "Notes" --format pdf --out ./notes.pdf
gog docs export <docId> --format txt --out - > doc.txt
gog docs list-tabs <docId>
gog docs cat <docId> --tab "Notes"
gog docs cat <docId> --all-tabs
gog docs update <docId> --text "Append this later"
gog docs update <docId> --text "Only in this tab" --tab "Notes"
gog docs update <docId> --file ./insert.txt --index 25 --pageless
gog docs write <docId> --text "Fresh content"
gog docs write <docId> --text "Rewrite one tab" --tab "Notes"
gog docs write <docId> --text "Important" --bold --text-color "#3366cc"
gog docs write <docId> --file ./body.txt --append --pageless
gog docs write <docId> --file ./body.md --replace --markdown
gog docs write <docId> --file ./body.md --append --markdown
gog docs format <docId> --match "Important" --bold --bg-color "#fff2cc"
gog docs format <docId> --match "todo" --match-all --no-bold --underline
gog docs format <docId> --alignment center --line-spacing 150
gog docs find-replace <docId> "old" "new"
gog docs find-replace <docId> "old" "new" --tab "Notes"
gog docs raw <docId>                                # Lossless JSON dump of Documents.Get (LLM/scripting)
gog docs raw <docId> --pretty

# Slides
gog slides info <presentationId>
gog slides create "My Deck"
gog slides create-from-markdown "My Deck" --content-file ./slides.md
gog slides create-from-template <templateId> "My Deck" --replace "name=John" --replace "date=2026-02-15"
gog slides copy <presentationId> "My Deck Copy"
gog slides export <presentationId> --format pdf --out ./deck.pdf
gog slides list-slides <presentationId>
gog slides add-slide <presentationId> ./slide.png --notes "Speaker notes"
gog slides update-notes <presentationId> <slideId> --notes "Updated notes"
gog slides replace-slide <presentationId> <slideId> ./new-slide.png --notes "New notes"
gog slides insert-text <presentationId> <objectId> "Text to insert"
gog slides insert-text <presentationId> <objectId> - < long-content.md
gog slides insert-text <presentationId> <objectId> "New body" --replace
gog slides replace-text <presentationId> "{{name}}" "Acme Corp"
gog slides replace-text <presentationId> "TODO" "DONE" --match-case --page <slideId1> --page <slideId2>
gog slides raw <presentationId>                     # Lossless JSON dump of Presentations.Get

# Sheets
gog sheets copy <spreadsheetId> "My Sheet Copy"
gog sheets export <spreadsheetId> --format pdf --out ./sheet.pdf
gog sheets format <spreadsheetId> 'Sheet1!A1:B2' --format-json '{"textFormat":{"bold":true}}' --format-fields 'userEnteredFormat.textFormat.bold'
gog sheets format <spreadsheetId> 'Sheet1!A1:B2' --format-json '{"borders":{"top":{"style":"SOLID"}}}' --format-fields 'userEnteredFormat.borders.top.style'
gog sheets merge <spreadsheetId> 'Sheet1!A1:B2'
gog sheets number-format <spreadsheetId> 'Sheet1!C:C' --type CURRENCY --pattern '$#,##0.00'
gog sheets freeze <spreadsheetId> --rows 1 --cols 1
gog sheets resize-columns <spreadsheetId> 'Sheet1!A:C' --auto
gog sheets read-format <spreadsheetId> 'Sheet1!A1:B2'
gog sheets insert <spreadsheetId> "Sheet1" rows 2 --count 3
gog sheets notes <spreadsheetId> 'Sheet1!A1:B10'
gog sheets find-replace <spreadsheetId> "old" "new"
gog sheets find-replace <spreadsheetId> "old" "new" --sheet Sheet1 --match-entire
gog sheets links <spreadsheetId> 'Sheet1!A1:B10'
gog sheets add-tab <spreadsheetId> <tabName> --index 0
gog sheets rename-tab <spreadsheetId> <oldName> <newName>
gog sheets delete-tab <spreadsheetId> <tabName> --force
gog sheets raw <spreadsheetId>                       # Lossless JSON dump of Spreadsheets.Get
gog sheets raw <spreadsheetId> --include-grid-data   # Include cell-level data (off by default)

# Other raw dumps (gmail, calendar, people, contacts, tasks, forms)
gog gmail raw <messageId>                            # Lossless JSON dump of Users.Messages.Get (default format=full)
gog gmail raw <messageId> --format raw               # Gmail's native format=raw (base64url RFC822)
gog calendar raw <calendarId> <eventId>              # Lossless JSON dump of Events.Get
gog people raw people/<resourceName>                 # Lossless JSON dump of People.Get
gog contacts raw people/<resourceName>               # Same endpoint, exposed under the contacts group
gog tasks raw <tasklistId> <taskId>                  # Lossless JSON dump of Tasks.Get
gog forms raw <formId>                               # Lossless JSON dump of Forms.Get
```

**Raw vs other read subcommands.** Use `raw` when you need the full
canonical Google API response as JSON (e.g. feeding a doc into an LLM,
or scripting against structural fields `cat`/`structure` drop). Other
read commands are lossier on purpose:

- `docs info`, `sheets metadata`, `slides info`, `drive get` → metadata only (cheap)
- `docs cat`, `docs structure` → plain text / simplified structure
- `docs export`, `sheets export`, `slides export`, `drive download` → converted file formats (pdf/docx/xlsx/pptx/md)
- `<group> raw` → full API response as JSON (verbose, lossless)

`drive raw` defaults to `fields=*` and redacts a small set of
capability/token-shaped fields (thumbnailLink, webContentLink,
exportLinks, resourceKey, appProperties, properties,
contentHints.thumbnail.image). When `--fields` is supplied the response
is returned verbatim — the user named the field, they get it. See
[`docs/raw-audit.md`](docs/raw-audit.md) for the redaction rationale.

### Contacts

```bash
# Personal contacts
gog contacts list --max 50
gog contacts search "Ada" --max 50
gog contacts get people/<resourceName>
gog contacts get user@example.com     # Get by email
gog contacts export user@example.com --out contact.vcf
gog contacts export --all --out contacts.vcf

# Other contacts (people you've interacted with)
gog contacts other list --max 50
gog contacts other search "John" --max 50

# Create and update
gog contacts create \
  --given "John" \
  --family "Doe" \
  --email "john@example.com" \
  --phone "+1234567890" \
  --address "12 St James's Square, London" \
  --gender "male" \
  --relation "spouse=Jane Doe"

gog contacts update people/<resourceName> \
  --given "Jane" \
  --email "jane@example.com" \
  --address "1 Infinite Loop, Cupertino" \
  --birthday "1990-05-12" \
  --gender "female" \
  --notes "Met at WWDC" \
  --relation "friend=Bob"

# Update via JSON (see docs/contacts-json-update.md)
gog contacts get people/<resourceName> --json | \
  jq '(.contact.urls //= []) | (.contact.urls += [{"value":"obsidian://open?vault=notes&file=People/John%20Doe","type":"profile"}])' | \
  gog contacts update people/<resourceName> --from-file -

gog contacts delete people/<resourceName>

# Workspace directory (requires Google Workspace)
gog contacts directory list --max 50
gog contacts directory search "Jane" --max 50
```

### Tasks

```bash
# Task lists
gog tasks lists --max 50
gog tasks lists create <title>

# Tasks in a list
gog tasks list <tasklistId> --max 50
gog tasks get <tasklistId> <taskId>
gog tasks add <tasklistId> --title "Task title"
gog tasks add <tasklistId> --title "Weekly sync" --due 2025-02-01 --repeat weekly --repeat-count 4
gog tasks add <tasklistId> --title "Daily standup" --due 2025-02-01 --repeat daily --repeat-until 2025-02-05
gog tasks add <tasklistId> --title "Bi-weekly review" --due 2025-02-01 --recur-rrule "FREQ=WEEKLY;INTERVAL=2" --repeat-count 3
gog tasks update <tasklistId> <taskId> --title "New title"
gog tasks done <tasklistId> <taskId>
gog tasks undo <tasklistId> <taskId>
gog tasks delete <tasklistId> <taskId>
gog tasks clear <tasklistId>

# Note: Google Tasks treats due dates as date-only; time components may be ignored.
# Note: Public Google Tasks API does not expose true recurring-task metadata; `--repeat*`/`--recur*` materialize concrete tasks.
# See docs/dates.md for all supported date/time input formats across commands.
```

### Sheets

```bash
# Read
gog sheets metadata <spreadsheetId>
gog sheets get <spreadsheetId> 'Sheet1!A1:B10'
gog sheets get <spreadsheetId> MyNamedRange

# Export (via Drive)
gog sheets export <spreadsheetId> --format pdf --out ./sheet.pdf
gog sheets export <spreadsheetId> --format xlsx --out ./sheet.xlsx

# Write
gog sheets update <spreadsheetId> 'A1' 'val1|val2,val3|val4'
gog sheets update <spreadsheetId> 'A1' --values-json '[["a","b"],["c","d"]]'
gog sheets update <spreadsheetId> 'Sheet1!A1:C1' 'new|row|data' --copy-validation-from 'Sheet1!A2:C2'
gog sheets update <spreadsheetId> MyNamedRange 'new|row|data'
gog sheets update <spreadsheetId> 'Sheet1!A1:C1' 'new|row|data' --copy-validation-from MyValidationNamedRange
gog sheets append <spreadsheetId> 'Sheet1!A:C' 'new|row|data'
gog sheets append <spreadsheetId> 'Sheet1!A:C' 'new|row|data' --copy-validation-from 'Sheet1!A2:C2'
gog sheets find-replace <spreadsheetId> "old" "new"
gog sheets find-replace <spreadsheetId> "old" "new" --sheet Sheet1 --regex
gog sheets update-note <spreadsheetId> 'Sheet1!A1' --note ''
gog sheets append <spreadsheetId> MyNamedRange 'new|row|data'
gog sheets clear <spreadsheetId> 'Sheet1!A1:B10'
gog sheets clear <spreadsheetId> MyNamedRange

# Format
gog sheets format <spreadsheetId> 'Sheet1!A1:B2' --format-json '{"textFormat":{"bold":true}}' --format-fields 'userEnteredFormat.textFormat.bold'
gog sheets format <spreadsheetId> MyNamedRange --format-json '{"textFormat":{"bold":true}}' --format-fields 'userEnteredFormat.textFormat.bold'
gog sheets format <spreadsheetId> 'Sheet1!A1:B2' --format-json '{"borders":{"top":{"style":"SOLID"}}}' --format-fields 'userEnteredFormat.borders.top.style'
gog sheets merge <spreadsheetId> 'Sheet1!A1:B2'
gog sheets unmerge <spreadsheetId> 'Sheet1!A1:B2'
gog sheets number-format <spreadsheetId> 'Sheet1!C:C' --type CURRENCY --pattern '$#,##0.00'
gog sheets freeze <spreadsheetId> --rows 1 --cols 1
gog sheets resize-columns <spreadsheetId> 'Sheet1!A:C' --auto
gog sheets resize-rows <spreadsheetId> 'Sheet1!1:10' --height 36
gog sheets read-format <spreadsheetId> 'Sheet1!A1:B2'
gog sheets read-format <spreadsheetId> 'Sheet1!A1:B2' --effective

# Named ranges
gog sheets named-ranges <spreadsheetId>
gog sheets named-ranges get <spreadsheetId> MyNamedRange
gog sheets named-ranges add <spreadsheetId> MyNamedRange 'Sheet1!A1:B2'
gog sheets named-ranges add <spreadsheetId> MyCols 'Sheet1!A:C'
gog sheets named-ranges update <spreadsheetId> MyNamedRange --name MyNamedRange2
gog sheets named-ranges delete <spreadsheetId> MyNamedRange2

# Tables
gog sheets table list <spreadsheetId>
gog sheets table create <spreadsheetId> 'Sheet1!A1:C4' --name Tasks --columns-json '[{"columnName":"Task","columnType":"TEXT"},{"columnName":"Amount","columnType":"DOUBLE"},{"columnName":"Done","columnType":"BOOLEAN"}]'
gog sheets table get <spreadsheetId> <tableId>
gog sheets table delete <spreadsheetId> <tableId> --force
# See docs/sheets-tables.md for valid column types and current command scope.

# Charts
gog sheets chart list <spreadsheetId>
gog sheets chart get <spreadsheetId> <chartId> --json > chart.json
gog sheets chart create <spreadsheetId> --spec-json @chart.json
gog sheets chart create <spreadsheetId> --spec-json '{"title":"Revenue","basicChart":{"chartType":"COLUMN"}}' --sheet Sheet1 --anchor E10
gog sheets chart update <spreadsheetId> <chartId> --spec-json '{"title":"New Title","basicChart":{"chartType":"PIE"}}'
gog sheets chart delete <spreadsheetId> <chartId>

# Insert rows/cols
gog sheets insert <spreadsheetId> "Sheet1" rows 2 --count 3
gog sheets insert <spreadsheetId> "Sheet1" cols 3 --after

# Notes
gog sheets notes <spreadsheetId> 'Sheet1!A1:B10'
gog sheets links <spreadsheetId> 'Sheet1!A1:B10'   # Includes rich-text links

# Create
gog sheets create "My New Spreadsheet" --sheets "Sheet1,Sheet2"

# Tab management
gog sheets add-tab <spreadsheetId> <tabName> --index 0
gog sheets rename-tab <spreadsheetId> <oldName> <newName>
gog sheets delete-tab <spreadsheetId> <tabName>          # use --force to skip confirmation
```

### Forms

```bash
# Forms
gog forms get <formId>
gog forms create --title "Weekly Check-in" --description "Friday async update"
gog forms update <formId> --title "Weekly Sync" --quiz true
gog forms add-question <formId> --title "What shipped?" --type paragraph --required
gog forms move-question <formId> 3 1
gog forms delete-question <formId> 2 --force

# Responses
gog forms responses list <formId> --max 20
gog forms responses get <formId> <responseId>

# Watches
gog forms watch create <formId> --topic projects/<project>/topics/<topic>
gog forms watch list <formId>
gog forms watch renew <formId> <watchId>
gog forms watch delete <formId> <watchId>
```

### Apps Script

```bash
# Projects
gog appscript get <scriptId>
gog appscript content <scriptId>
gog appscript create --title "Automation Helpers"
gog appscript create --title "Bound Script" --parent-id <driveFileId>

# Execute functions
gog appscript run <scriptId> myFunction --params '["arg1", 123, true]'
gog appscript run <scriptId> myFunction --dev-mode
```

### People

```bash
# Profile
gog people me
gog people get people/<userId>

# Search the Workspace directory
gog people search "Ada Lovelace" --max 5

# Relations (defaults to people/me)
gog people relations
gog people relations people/<userId> --type manager
```

### Chat

```bash
# Spaces
gog chat spaces list
gog chat spaces find "Engineering"
gog chat spaces find "Engineering" --exact
gog chat spaces create "Engineering" --member alice@company.com --member bob@company.com

# Messages
gog chat messages list spaces/<spaceId> --max 5
gog chat messages list spaces/<spaceId> --thread <threadId>
gog chat messages list spaces/<spaceId> --unread
gog chat messages send spaces/<spaceId> --text "Build complete!" --thread spaces/<spaceId>/threads/<threadId>
gog chat messages reactions list spaces/<spaceId>/messages/<messageId>
gog chat messages react spaces/<spaceId>/messages/<messageId> "👍"  # shorthand for reactions create
gog chat messages reactions delete spaces/<spaceId>/messages/<messageId>/reactions/<reactionId>

# Threads
gog chat threads list spaces/<spaceId>

# Direct messages
gog chat dm space user@company.com
gog chat dm send user@company.com --text "ping"
```

Note: Chat commands require a Google Workspace account (consumer @gmail.com accounts are not supported).

### Admin

```bash
# Requires a Workspace service account with domain-wide delegation.
gog admin users list --domain example.com
gog admin users get user@example.com
gog admin users create user@example.com --given Ada --family Lovelace --password 'TempPass123!'
gog admin users suspend user@example.com --force

gog admin groups list --domain example.com
gog admin groups members list engineering@example.com
gog admin groups members add engineering@example.com user@example.com --role MEMBER
gog admin groups members remove engineering@example.com user@example.com --force
```

### Groups (Google Workspace)

```bash
# List groups you belong to
gog groups list

# List members of a group
gog groups members engineering@company.com
```

Note: Groups commands require the Cloud Identity API and the `cloud-identity.groups.readonly` scope. If you get a permissions error, re-authenticate:

```bash
gog auth add your@email.com --services groups --force-consent
```

### Classroom (Google Workspace for Education)

```bash
# Courses
gog classroom courses list
gog classroom courses list --role teacher
gog classroom courses get <courseId>
gog classroom courses create --name "Math 101"
gog classroom courses update <courseId> --name "Math 102"
gog classroom courses archive <courseId>
gog classroom courses unarchive <courseId>
gog classroom courses url <courseId>

# Roster
gog classroom roster <courseId>
gog classroom roster <courseId> --students
gog classroom students add <courseId> <userId>
gog classroom teachers add <courseId> <userId>

# Coursework
gog classroom coursework list <courseId>
gog classroom coursework get <courseId> <courseworkId>
gog classroom coursework create <courseId> --title "Homework 1" --type ASSIGNMENT --state PUBLISHED
gog classroom coursework update <courseId> <courseworkId> --title "Updated"
gog classroom coursework assignees <courseId> <courseworkId> --mode INDIVIDUAL_STUDENTS --add-student <studentId>

# Materials
gog classroom materials list <courseId>
gog classroom materials create <courseId> --title "Syllabus" --state PUBLISHED

# Submissions
gog classroom submissions list <courseId> <courseworkId>
gog classroom submissions get <courseId> <courseworkId> <submissionId>
gog classroom submissions grade <courseId> <courseworkId> <submissionId> --grade 85
gog classroom submissions return <courseId> <courseworkId> <submissionId>
gog classroom submissions turn-in <courseId> <courseworkId> <submissionId>
gog classroom submissions reclaim <courseId> <courseworkId> <submissionId>

# Announcements
gog classroom announcements list <courseId>
gog classroom announcements create <courseId> --text "Welcome!"
gog classroom announcements update <courseId> <announcementId> --text "Updated"
gog classroom announcements assignees <courseId> <announcementId> --mode INDIVIDUAL_STUDENTS --add-student <studentId>

# Topics
gog classroom topics list <courseId>
gog classroom topics create <courseId> --name "Unit 1"
gog classroom topics update <courseId> <topicId> --name "Unit 2"

# Invitations
gog classroom invitations list
gog classroom invitations create <courseId> <userId> --role student
gog classroom invitations accept <invitationId>

# Guardians
gog classroom guardians list <studentId>
gog classroom guardians get <studentId> <guardianId>
gog classroom guardians delete <studentId> <guardianId>

# Guardian invitations
gog classroom guardian-invitations list <studentId>
gog classroom guardian-invitations create <studentId> --email parent@example.com

# Profiles
gog classroom profile get
gog classroom profile get <userId>
```

Note: Classroom commands require a Google Workspace for Education account. Personal Google accounts have limited Classroom functionality.

### Docs

```bash
# Export (via Drive)
gog docs export <docId> --format pdf --out ./doc.pdf
gog docs export <docId> --format docx --out ./doc.docx
gog docs export <docId> --format txt --out ./doc.txt
gog docs export <docId> --format md --out ./doc.md
gog docs export <docId> --format html --out ./doc.html
gog docs export <docId> --format txt --out - > doc.txt
gog docs export <docId> --tab "Notes" --format md --out ./notes.md
```

`docs export` uses Drive export for whole-document downloads. `--tab` is experimental: Google Drive cannot export a single Docs tab, so gog resolves the tab title or ID with the Docs API and downloads that tab through Google's undocumented Docs web export endpoint. Use `gog docs list-tabs <docId>` to see tab titles/IDs. Tab export supports `pdf`, `docx`, `txt`, `md`, and `html`, including `--out -`; if Google changes that web endpoint, gog fails before writing sign-in HTML or unrelated redirect content.

```bash
# Sed-style regex editing with Markdown formatting (sedmat)
gog docs sed <docId> 's/pattern/replacement/g'

# Formatting in replacements
gog docs sed <docId> 's/hello/**hello**/'          # bold
gog docs sed <docId> 's/hello/*hello*/'             # italic
gog docs sed <docId> 's/hello/~~hello~~/'           # strikethrough
gog docs sed <docId> 's/hello/`hello`/'             # monospace
gog docs sed <docId> 's/hello/__hello__/'           # underline
gog docs sed <docId> 's/Google/[Google](https://google.com)/'  # link

# Images
gog docs sed <docId> 's/{{LOGO}}/![](https://example.com/logo.png)/'
gog docs sed <docId> 's/{{HERO}}/![](https://example.com/hero.jpg){width=600}/'

# Tables — create, populate, modify
gog docs sed <docId> 's/{{TABLE}}/|3x4|/'            # create 3-row, 4-col table
gog docs sed <docId> 's/|1|[A1]/**Name**/'           # set cell A1 (bold)
gog docs sed <docId> 's/|1|[1,*]/**&**/'             # bold entire row 1
gog docs sed <docId> 's/|1|[row:+2]//'               # insert row before row 2
gog docs sed <docId> 's/|1|[col:$+]//'               # append column at end
```

> See [docs/sedmat.md](docs/sedmat.md) for the full sedmat syntax reference.

### Slides

```bash
# Export (via Drive)
gog slides export <presentationId> --format pptx --out ./deck.pptx
gog slides export <presentationId> --format pdf --out ./deck.pdf

# Create from template with text replacements
gog slides create-from-template <templateId> "Q1 Report" \
  --replace "quarter=Q1 2026" \
  --replace "revenue=$1.2M" \
  --replace "growth=15%"

# Create from Markdown
cat > slides.md <<'EOF'
## Roadmap

- Ship auth migration
- Polish backup restore

---

## Launch Notes

Short paragraphs become body text.
EOF

gog slides create-from-markdown "Roadmap" --content-file ./slides.md

# Use JSON file for many replacements
cat > replacements.json <<EOF
{
  "name": "John Doe",
  "title": "Sales Manager",
  "date": "2026-02-15",
  "sales": "125",
  "target": "100"
}
EOF

gog slides create-from-template <templateId> "Monthly Report" \
  --replacements replacements.json

# Read slide content (text, notes, images)
gog slides read-slide <presentationId> <slideId>

# Include grouped elements, word art, and tables
gog slides read-slide <presentationId> <slideId> --recursive --json

# Get a rendered slide thumbnail URL
gog slides thumbnail <presentationId> <slideId>

# Download a rendered slide thumbnail
gog slides thumbnail <presentationId> <slideId> --output ./slide.png

# Control thumbnail size and format
gog slides thumbnail <presentationId> <slideId> --size medium --format jpeg --output ./slide.jpg

# Insert text into an existing text-capable element (shape or table cell)
gog slides insert-text <presentationId> <objectId> "Hello, world"

# Insert at a specific position in the element's existing text
gog slides insert-text <presentationId> <objectId> " (inserted)" --insertion-index 12

# Replace the element's existing text wholesale (DeleteText + InsertText in one batch)
gog slides insert-text <presentationId> <objectId> "Brand-new body copy" --replace

# Read long content from stdin
cat long-content.md | gog slides insert-text <presentationId> <objectId> -

# Preview the batchUpdate request body without executing it
gog slides insert-text <presentationId> <objectId> "demo" --replace --dry-run

# Find-and-replace across the whole deck
gog slides replace-text <presentationId> "{{customer_name}}" "Acme Corp"

# Case-sensitive match, restricted to specific slides
gog slides replace-text <presentationId> "TODO" "DONE" \
  --match-case \
  --page <slideId1> --page <slideId2>

# Preview the replace request without executing it
gog slides replace-text <presentationId> "old" "new" --dry-run
```

`slides create-from-markdown` uses `---` lines as slide separators and `##` headings as slide titles. It supports bullets, paragraphs, and fenced code blocks; see [docs/slides-markdown.md](docs/slides-markdown.md).

## Output Formats

### Text

Human-readable output with colors (default):

```bash
$ gog gmail search 'newer_than:7d' --max 3
THREAD_ID           SUBJECT                           FROM                  DATE
18f1a2b3c4d5e6f7    Meeting notes                     alice@example.com     2025-01-10
17e1d2c3b4a5f6e7    Invoice #12345                    billing@vendor.com    2025-01-09
16d1c2b3a4e5f6d7    Project update                    bob@example.com       2025-01-08
```

Message-level search (one row per email; add `--include-body` to fetch/decode bodies, `--body-format html` to prefer HTML bodies, or `--full` for untruncated text output):

```bash
$ gog gmail messages search 'newer_than:7d' --max 3
ID                  THREAD             SUBJECT                           FROM                  DATE
18f1a2b3c4d5e6f7    9e8d7c6b5a4f3e2d    Meeting notes                     alice@example.com     2025-01-10
17e1d2c3b4a5f6e7    9e8d7c6b5a4f3e2d    Invoice #12345                    billing@vendor.com    2025-01-09
16d1c2b3a4e5f6d7    7f6e5d4c3b2a1908    Project update                    bob@example.com       2025-01-08
```

```bash
$ gog gmail messages search 'from:newsletter@example.com' --include-body --body-format html --json
```

### JSON

Machine-readable output for scripting and automation:

```bash
$ gog gmail search 'newer_than:7d' --max 3 --json
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
$ gog gmail messages search 'newer_than:7d' --max 3 --json
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
$ gog gmail messages search 'newer_than:7d' --max 1 --full --json
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
gog --json drive ls --max 5 | jq '.files[] | select(.mimeType=="application/pdf")'
```

Useful pattern:

- `gog --json ... | jq .`

Calendar JSON convenience fields:

- `startDayOfWeek` / `endDayOfWeek` on event payloads (derived from start/end).

## Examples

### Search recent emails and download attachments

```bash
# Search for emails from the last week
gog gmail search 'newer_than:7d has:attachment' --max 10

# Get thread details and download attachments
gog gmail thread get <threadId> --download
```

### Modify labels on a thread

```bash
# Archive and star a thread
gog gmail thread modify <threadId> --remove INBOX --add STARRED
```

### Create a calendar event with attendees

```bash
# Find a free time slot
gog calendar freebusy --calendars "primary" \
  --from 2025-01-15T00:00:00Z \
  --to 2025-01-16T00:00:00Z

# Create the meeting
gog calendar create primary \
  --summary "Team Standup" \
  --from 2025-01-15T10:00:00Z \
  --to 2025-01-15T10:30:00Z \
  --attendees "alice@example.com,bob@example.com"
```

### Find and download files from Drive

```bash
# Search for PDFs
gog drive search "invoice filetype:pdf" --max 20 --json | \
  jq -r '.files[] | .id' | \
  while read fileId; do
    gog drive download "$fileId"
  done
```

### Manage multiple accounts

```bash
# Check personal Gmail
gog gmail search 'is:unread' --account personal@gmail.com

# Check work Gmail
gog gmail search 'is:unread' --account work@company.com

# Or set default
export GOG_ACCOUNT=work@company.com
gog gmail search 'is:unread'
```

### Update a Google Sheet from a CSV

```bash
# Convert CSV to pipe-delimited format and update sheet
cat data.csv | tr ',' '|' | \
  gog sheets update <spreadsheetId> 'Sheet1!A1'
```

### Export Sheets / Docs / Slides

```bash
# Sheets
gog sheets export <spreadsheetId> --format pdf

# Docs
gog docs export <docId> --format docx

# Slides
gog slides export <presentationId> --format pptx
```

### Batch process Gmail threads

```bash
# Mark all emails from a sender as read
gog --json gmail search 'from:noreply@example.com' --max 200 | \
  jq -r '.threads[].id' | \
  xargs -n 50 gog gmail labels modify --remove UNREAD

# Archive old emails
gog --json gmail search 'older_than:1y' --max 200 | \
  jq -r '.threads[].id' | \
  xargs -n 50 gog gmail labels modify --remove INBOX

# Label important emails
gog --json gmail search 'from:boss@example.com' --max 200 | \
  jq -r '.threads[].id' | \
  xargs -n 50 gog gmail labels modify --add IMPORTANT
```

## Advanced Features

### Verbose Mode

Enable verbose logging for troubleshooting:

```bash
gog --verbose gmail search 'newer_than:7d'
# Shows API requests and responses
```

## Global Flags

All commands support these flags:

- `--account <email|alias|auto>` - Account to use (overrides GOG_ACCOUNT)
- `--enable-commands <csv>` - Allowlist commands; dot paths allowed (e.g., `calendar,tasks,gmail.search`)
- `--disable-commands <csv>` - Denylist commands; dot paths allowed (e.g., `gmail.send,gmail.drafts.send`)
- `--gmail-no-send` - Block Gmail send operations
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
gog completion bash > $(brew --prefix)/etc/bash_completion.d/gog

# Linux
gog completion bash > /etc/bash_completion.d/gog

# Or load directly in your current session
source <(gog completion bash)
```

### Zsh

```zsh
# Generate completion file
gog completion zsh > "${fpath[1]}/_gog"

# Or add to .zshrc for automatic loading
echo 'eval "$(gog completion zsh)"' >> ~/.zshrc

# Enable completions if not already enabled
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

### Fish

```fish
gog completion fish > ~/.config/fish/completions/gog.fish
```

### PowerShell

```powershell
# Load for current session
gog completion powershell | Out-String | Invoke-Expression

# Or add to profile for all sessions
gog completion powershell >> $PROFILE
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

Documentation is split between hand-written topic pages in `docs/` and generated
per-command pages in `docs/commands/`. Every CLI command should have a docs page;
do not hand-edit generated command pages. Regenerate them from the live schema
whenever command names, flags, aliases, arguments, or help text change:

```bash
make docs-commands
```

Build the GitHub Pages site locally:

```bash
make docs-site
open dist/docs-site/index.html
```

The Pages workflow rebuilds the command pages and publishes the static site from
`dist/docs-site` using the custom domain in `docs/CNAME`.

### Integration Tests (Live Google APIs)

Opt-in tests that hit real Google APIs using your stored `gog` credentials/tokens.

```bash
# Optional: override which account to use
export GOG_IT_ACCOUNT=you@gmail.com
export GOG_CLIENT=work
go test -tags=integration ./...
```

Tip: if you want to avoid macOS Keychain prompts during these runs, set `GOG_KEYRING_BACKEND=file` and `GOG_KEYRING_PASSWORD=...` (uses encrypted on-disk keyring).

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
GOG_LIVE=1 go test -tags=integration ./internal/integration -run Live
```

Optional env:
- `GOG_LIVE_FAST=1`
- `GOG_LIVE_SKIP=groups,keep`
- `GOG_LIVE_AUTH=all,groups`
- `GOG_LIVE_ALLOW_NONTEST=1`
- `GOG_LIVE_EMAIL_TEST=steipete+gogtest@gmail.com`
- `GOG_LIVE_GROUP_EMAIL=group@domain`
- `GOG_LIVE_CLASSROOM_COURSE=<courseId>`
- `GOG_LIVE_CLASSROOM_CREATE=1`
- `GOG_LIVE_CLASSROOM_ALLOW_STATE=1`
- `GOG_LIVE_TRACK=1`
- `GOG_LIVE_GMAIL_BATCH_DELETE=1`
- `GOG_LIVE_GMAIL_FILTERS=1`
- `GOG_LIVE_GMAIL_WATCH_TOPIC=projects/.../topics/...`
- `GOG_LIVE_CALENDAR_RESPOND=1`
- `GOG_LIVE_CALENDAR_RECURRENCE=1`
- `GOG_KEEP_SERVICE_ACCOUNT=/path/to/service-account.json`
- `GOG_KEEP_IMPERSONATE=user@workspace-domain`

### Make Shortcut

Build and run:

```bash
make gog auth add you@gmail.com
```

For clean stdout when scripting:

- Use `--` when the first arg is a flag: `make gog -- --json gmail search "from:me" | jq .`

## License

MIT

## Links

- [GitHub Repository](https://github.com/steipete/gogcli)
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
