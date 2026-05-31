# gogcli

![gogcli banner](docs/assets/readme-banner.jpg)

`gog` is a script-friendly Google CLI for Gmail, Calendar, Drive, Docs, Sheets,
Sites, Slides, Forms, Meet, Apps Script, Analytics, Search Console, Contacts,
Tasks, People, Classroom, Chat, YouTube, and Workspace admin flows.

It is built for terminals, shell scripts, CI, and coding agents:

- predictable `--json` and `--plain` output on stdout
- human hints and progress on stderr
- multiple Google accounts and OAuth clients
- OAuth, direct access tokens, ADC, and Workspace service accounts
- runtime command allowlists/denylists and baked safety-profile binaries
- typed [MCP server](docs/mcp.md) for agent clients, read-only by default and
  without a generic command runner
- read-only audit/reporting commands for risky surfaces like Drive and Contacts
- [generated docs](docs/commands/README.md) for every command

Rendered docs: <https://gogcli.sh/>

Start here:

- [Install](docs/install.md)
- [Quickstart](docs/quickstart.md)
- [Auth clients and service accounts](docs/auth-clients.md)
- [MCP server](docs/mcp.md)
- [Command index](docs/commands/README.md)
- [Gmail watch / Pub/Sub push](docs/watch.md) (<https://gogcli.sh/watch.html>)

## Install

See the full [Install](docs/install.md) guide for Homebrew, Docker, Windows
ZIPs, source builds, and headless/container keyring setup.

### Homebrew

```bash
brew install openclaw/tap/gogcli
gog --version
```

### Docker

```bash
docker run --rm ghcr.io/openclaw/gogcli:latest version
```

Authenticated container runs should use a persistent config volume and the
encrypted file keyring:

```bash
docker volume create gogcli-config

docker run --rm -it \
  -e GOG_KEYRING_BACKEND=file \
  -e GOG_KEYRING_PASSWORD \
  -v gogcli-config:/home/gog/.config/gogcli \
  ghcr.io/openclaw/gogcli:latest \
  auth add you@gmail.com --services gmail,calendar,drive
```

### Windows

Download `gogcli_<version>_windows_amd64.zip` or
`gogcli_<version>_windows_arm64.zip` from the
[latest release](https://github.com/openclaw/gogcli/releases), extract
`gog.exe`, and put that directory on `PATH`.

### Build from source

```bash
git clone https://github.com/openclaw/gogcli.git
cd gogcli
make
./bin/gog --version
```

Source builds require the Go version declared in `go.mod`.

## Quick Start

The full walkthrough lives in [Quickstart](docs/quickstart.md). For named OAuth
clients, remote OAuth, direct access tokens, ADC, and Workspace service
accounts, see [Auth clients](docs/auth-clients.md).

Create a Google Cloud project, enable the APIs you need, create a Desktop OAuth
client, then store that client JSON in `gog`.

```bash
gog auth credentials ~/Downloads/client_secret_....json
gog auth add you@gmail.com --services gmail,calendar,drive,docs,sheets,contacts
gog auth doctor --check

export GOG_ACCOUNT=you@gmail.com
gog gmail search 'newer_than:7d' --max 10
```

Useful Google setup links:

- [Create a Cloud project](https://console.cloud.google.com/projectcreate)
- [OAuth clients](https://console.cloud.google.com/auth/clients)
- [OAuth consent screen](https://console.cloud.google.com/auth/branding)
- [API library](https://console.cloud.google.com/apis/library)
- [Places API (New)](https://console.cloud.google.com/apis/api/places.googleapis.com)
- [Drive Labels API](https://console.cloud.google.com/apis/api/drivelabels.googleapis.com)
- [Photos Library API](https://console.cloud.google.com/apis/api/photoslibrary.googleapis.com)
- [Google Analytics Admin API](https://console.cloud.google.com/apis/api/analyticsadmin.googleapis.com)
- [Google Analytics Data API](https://console.cloud.google.com/apis/api/analyticsdata.googleapis.com)
- [Google Search Console API](https://console.cloud.google.com/apis/api/searchconsole.googleapis.com)
- [YouTube Data API v3](https://console.cloud.google.com/apis/api/youtube.googleapis.com)
- [Apps Script user setting](https://script.google.com/home/usersettings)

Enable APIs in the same Cloud project that owns your OAuth client. If Google
returns `accessNotConfigured`, enable that API and retry after propagation.

Consumer `gmail.com` accounts work for normal user APIs such as Gmail, Calendar,
Drive, Docs, Sheets, Slides, Forms, Apps Script, Analytics, Search Console,
Contacts/People, Tasks, and Classroom. Workspace-only APIs such as Admin
Directory, Cloud Identity Groups, Chat, and Keep/domain-wide-delegation flows
require a managed domain.

If your OAuth app is External + Testing, Google refresh tokens for user-data
scopes can expire after 7 days. Publish the personal OAuth app if you want
long-lived refresh tokens.

## Daily Examples

Every command below has generated reference docs in the
[Command index](docs/commands/README.md). The feature guides linked under each
section explain the workflow shape and safety notes.

### Gmail

Docs: [Gmail workflows](docs/gmail-workflows.md),
[Gmail watch](docs/watch.md), [email tracking](docs/email-tracking.md),
[`gog gmail`](docs/commands/gog-gmail.md).

```bash
# Search mail and get sanitized message content for agents/scripts.
gog gmail search 'from:boss newer_than:30d' --json
gog gmail get <messageId> --sanitize-content --json

# Export Gmail filters in the format the Gmail web UI can import.
gog gmail settings filters export --out filters.xml

# Hard block send operations during automation.
gog --gmail-no-send gmail drafts create --to you@example.com --subject test
```

### Calendar

Docs: [`gog calendar`](docs/commands/gog-calendar.md),
[`calendar create`](docs/commands/gog-calendar-create.md),
[`calendar update`](docs/commands/gog-calendar-update.md),
[`calendar move`](docs/commands/gog-calendar-move.md),
[Zoom setup](docs/zoom-auth-setup.md).

```bash
gog calendar events --today
gog calendar create --summary "Review" \
  --from "2026-05-06T10:00:00+02:00" \
  --to "2026-05-06T10:30:00+02:00"
gog calendar create primary --summary "Coffee" \
  --from "2026-05-06T10:00:00+02:00" \
  --to "2026-05-06T10:30:00+02:00" \
  --location-search "Elysian Coffee Vancouver"
gog calendar update primary <eventId> --with-meet
gog zoom auth setup
gog calendar create primary --summary "Client sync" \
  --from "2026-05-06T11:00:00+02:00" \
  --to "2026-05-06T11:30:00+02:00" \
  --with-zoom
gog calendar move primary <eventId> team-calendar@example.com
gog calendar appointments
```

### Drive

Docs: [Drive audits](docs/drive-audits.md), [raw API dumps](docs/raw-api.md),
[`gog drive`](docs/commands/gog-drive.md),
[`drive changes`](docs/commands/gog-drive-changes.md),
[`drive activity`](docs/commands/gog-drive-activity.md).

```bash
# Read-only folder audits.
gog drive tree --parent <folderId> --depth 2
gog drive du --parent <folderId> --max 20 --json
gog drive inventory --parent <folderId> --json
gog drive audit sharing --parent <folderId> --internal-domain example.com --json
gog drive audit user clawdbot@gmail.com --parent <folderId> --json
gog drive bulk remove-public --parent <folderId> --dry-run
gog drive share <fileId> --to user --email person@example.com --notify --dry-run
gog drive labels list --json
gog drive labels file list <fileId> --json
gog drive labels file apply <fileId> <labelId> --text fieldId=value
# Drive Labels requires a Google Workspace customer.

# Ask Drive for non-default fields.
gog drive get <fileId> --fields 'id,name,mimeType,size,owners,emailAddress' --json

# Track changes and audit activity.
gog drive changes start-token
gog drive changes list --token <token> --json
gog drive activity query --file <fileId> --actions edit,share --from 2026-01-01T00:00:00Z --json

# Lossless raw API JSON.
gog drive raw <fileId> --pretty
```

### Maps

Docs: [`gog maps`](docs/commands/gog-maps.md),
[`maps places`](docs/commands/gog-maps-places.md).

```bash
gog maps places search "Elysian Coffee Vancouver" --json
gog maps places details <placeId> --json
gog maps directions --origin "Vancouver, BC" --destination "Seattle, WA" --json
gog maps distance --origins "Vancouver BC" --destinations "Seattle WA" --json
gog maps geocode "1600 Amphitheatre Parkway, Mountain View, CA" --json
gog maps reverse-geocode --lat=37.422 --lng=-122.084 --json
```

Use comma-separated `maps distance --origins/--destinations` for multiple
locations. If an address itself contains commas, pass it without commas or use a
Place ID/lat,lng value to avoid splitting it.

### Photos

Docs: [`gog photos`](docs/commands/gog-photos.md).

Google Photos Library API access is limited to app-created media through
`photoslibrary.readonly.appcreateddata`, and the Photos Library API must be
enabled on the OAuth project used for `gog auth add`.

```bash
gog photos list --json
gog photos search --media-type PHOTO --from 2026-01-01 --to 2026-01-31 --json
gog photos download <mediaItemId> --out photo.jpg
```

### Contacts

Docs: [contacts dedupe](docs/contacts-dedupe.md),
[JSON contact updates](docs/contacts-json-update.md),
[`gog contacts`](docs/commands/gog-contacts.md).

```bash
gog contacts search alice --json
gog contacts export --all --out contacts.vcf

# Preview only: no merge/delete/update call is made.
gog contacts dedupe --json
gog contacts dedupe --match email,phone,name --dry-run
```

### Docs

Docs: [Google Docs editing](docs/docs-editing.md),
[sed-style document edits](docs/sedmat.md),
[`gog docs`](docs/commands/gog-docs.md).

```bash
gog docs write <docId> --append --markdown --text '## Status'
gog docs format <docId> --match Status --bold --font-size 18
gog docs insert-page-break <docId> --at-end
gog docs insert-table <docId> --rows 3 --cols 2 --at-end
gog docs add-tab <docId> --title "Notes"
gog docs tabs add <docId> --title "Notes"
gog docs find-replace <docId> old new --tab "Notes" --dry-run
gog docs raw <docId> --pretty
```

### Sheets

Docs: [Sheets batch updates](docs/sheets-batch-update.md),
[Sheets tables](docs/sheets-tables.md),
[Sheets formatting](docs/sheets-formatting.md),
[`gog sheets`](docs/commands/gog-sheets.md).

```bash
gog sheets get <spreadsheetId> 'Sheet1!A1:D20' --json
gog sheets batch-update <spreadsheetId> --data-json @updates.json --json
gog sheets table list <spreadsheetId>
gog sheets table append <spreadsheetId> Tasks 'Ship README|done'
gog sheets table clear <spreadsheetId> Tasks
gog sheets conditional-format add <spreadsheetId> 'Sheet1!A2:A100' \
  --type text-contains \
  --expr blocked \
  --format-json '{"backgroundColor":{"red":1,"green":0.84,"blue":0.84}}'
gog sheets banding set <spreadsheetId> 'Sheet1!A1:D100'
```

### Slides and Forms

Docs: [Slides from Markdown](docs/slides-markdown.md),
[template replacement](docs/slides-template-replacement.md),
[`gog slides`](docs/commands/gog-slides.md),
[`gog forms`](docs/commands/gog-forms.md).

```bash
gog slides create-from-markdown "Weekly update" --content-file slides.md
gog slides insert-text <presentationId> <objectId> "New text"
gog forms update <formId> --quiz=true
gog forms add-question <formId> --title "What is 2+2?" --type radio -o 1 -o 4 --correct 4 --points 1
gog forms questions add <formId> --title "What is 2+2?" --type radio -o 1 -o 4 --correct 4 --points 1
gog forms publish <formId>
gog forms responses list <formId> --json
gog forms raw <formId> --pretty
```

### YouTube

Docs: [`gog youtube`](docs/commands/gog-youtube.md),
[`youtube channels`](docs/commands/gog-youtube-channels.md),
[`youtube videos`](docs/commands/gog-youtube-videos.md),
[`youtube activities`](docs/commands/gog-youtube-activities.md).

```bash
gog config set youtube_api_key YOUR_API_KEY
gog yt channels list --id UC_x5XG1OV2P6uZZ5FSM9Ttw --json
gog yt videos list --chart mostPopular --region US --max 5
gog yt activities list --mine -a you@gmail.com
```

For API-key reads, enable YouTube Data API v3 on the key's Google Cloud project
and make sure API-key restrictions allow YouTube Data API calls. Authenticated
`--mine` reads use OAuth instead.

### Analytics and Search Console

Docs: [`gog analytics`](docs/commands/gog-analytics.md),
[`analytics report`](docs/commands/gog-analytics-report.md),
[`gog searchconsole`](docs/commands/gog-searchconsole.md),
[`searchconsole query`](docs/commands/gog-searchconsole-query.md).

```bash
gog analytics accounts --all --json
gog analytics report 123456789 --from 7daysAgo --to today --dimensions date,country --metrics activeUsers,sessions
gog searchconsole sites
gog searchconsole query sc-domain:example.com --from 2026-02-01 --to 2026-02-07 --dimensions query,page --filter query:contains:gog
gog searchconsole sitemaps submit sc-domain:example.com https://example.com/sitemap.xml --force
```

### Backup

Docs: [Backup](docs/backup.md), [`gog backup`](docs/commands/gog-backup.md).

```bash
gog backup init --repo ~/Backups/gog
gog backup push --services gmail,calendar,contacts,drive
gog backup verify
gog backup export --gmail-format markdown --out ~/Exports/gog
```

See [docs/backup.md](docs/backup.md) before running broad or unattended backup
jobs.

## Output and Automation

Docs: [Safety Profiles](docs/safety-profiles.md),
[`gog schema`](docs/commands/gog-schema.md),
[`gog config no-send`](docs/commands/gog-config-no-send.md).

Use `--json` for structured output and `--plain` for stable TSV. Prompts,
progress, and warnings go to stderr so stdout stays parseable.

```bash
gog --json gmail search 'has:attachment newer_than:90d' --max 50 |
  jq -r '.threads[].id'

gog --plain calendar events --today
```

Useful global flags:

- `--account <email|alias|auto>`: select an account
- `--client <name>`: select a stored OAuth client
- `--json`: JSON stdout
- `--plain`: stable parseable text stdout
- `--wrap-untrusted`: in JSON/raw output, wrap fetched free-text fields with
  external untrusted-content markers for LLM/agent consumption
- `--dry-run`: print intended actions where a command supports planning
- `--no-input`: fail instead of prompting
- `--force`: confirm destructive operations
- `--enable-commands <csv>`: allow selected command prefixes. Parent paths allow children, so `gmail` allows the Gmail command family.
- `--enable-commands-exact <csv>`: allow only exact command paths. Parent paths do not allow children, so `gmail.search` allows `gog gmail search` without allowing sibling commands like `gog gmail send`.
- `--disable-commands <csv>`: block selected command paths
- `--gmail-no-send`: block Gmail send operations

For coding agents or CI, prefer:

```bash
gog --account you@gmail.com \
  --enable-commands-exact gmail.search,gmail.get,drive.ls,docs.cat \
  --gmail-no-send \
  --wrap-untrusted \
  --json \
  gmail search 'newer_than:7d'
```

For environment-configured agents or CI, set `GOG_ENABLE_COMMANDS_EXACT` to the
same comma-separated exact command paths:

```bash
GOG_ENABLE_COMMANDS_EXACT=gmail.search,gmail.get,drive.ls,docs.cat \
GOG_GMAIL_NO_SEND=1 \
GOG_WRAP_UNTRUSTED=1 \
gog --json gmail search 'newer_than:7d'
```

For stricter agent deployments, build or download a baked safety-profile binary.
See [docs/safety-profiles.md](docs/safety-profiles.md).

### MCP server

`gog mcp` exposes a typed MCP stdio server for agent clients. It registers
specific Google tools such as `gmail_search`, `docs_get`, and
`sheets_read_range`; it does not expose a generic `gog_exec` or arbitrary
command bridge.

```bash
# Read-only server.
gog --account you@gmail.com mcp

# Docs tools only; writes require explicit opt-in.
gog --account you@gmail.com \
  --enable-commands-exact docs.cat,docs.write \
  mcp \
  --allow-write \
  --allow-tool docs.*
```

See [docs/mcp.md](docs/mcp.md) for client config, tool selection, safety
behavior, mcporter examples, and troubleshooting.

## Auth and Accounts

Docs: [Auth clients](docs/auth-clients.md),
[`gog auth`](docs/commands/gog-auth.md),
[`auth add`](docs/commands/gog-auth-add.md),
[`auth doctor`](docs/commands/gog-auth-doctor.md),
[`auth service-account`](docs/commands/gog-auth-service-account.md).

### OAuth clients

Store a Desktop OAuth client once:

```bash
gog auth credentials ~/Downloads/client_secret_....json
gog auth add you@gmail.com --services gmail,calendar,drive
```

Use named clients when different accounts should use different Cloud projects:

```bash
gog --client work auth credentials ~/Downloads/work-client.json
gog --client work auth add you@company.com
gog auth credentials list
```

See [docs/auth-clients.md](docs/auth-clients.md) for client selection rules,
domain mapping, remote OAuth, direct access tokens, ADC, and service accounts.

### Account selection

```bash
gog auth list --check
gog auth alias set work you@company.com

gog --account work gmail search 'is:unread'
export GOG_ACCOUNT=you@gmail.com
gog calendar events --today
```

### Keyring backends

By default `gog` uses the best OS keyring available. For headless or container
runs, use the encrypted file backend and inject `GOG_KEYRING_PASSWORD` from the
current shell or secret store.

```bash
gog auth keyring
gog auth keyring file
GOG_KEYRING_BACKEND=file GOG_KEYRING_PASSWORD=... gog auth list --check
```

For systemd services, gateways, and coding agents, set the same variables on
the service or agent process itself. A successful shell check does not mean the
agent subprocess inherited `GOG_KEYRING_PASSWORD`; verify through the actual
agent entrypoint with `gog auth doctor --check --no-input`.

Use `GOG_HOME=/persist/gogcli` to keep config, data, state, and cache under one
portable root, or set `GOG_CONFIG_DIR`, `GOG_DATA_DIR`, `GOG_STATE_DIR`, and
`GOG_CACHE_DIR` individually for split lifetimes. These overrides are above XDG
paths and are useful in containers, CI, and agent sandboxes.

Never commit OAuth client JSON files, refresh tokens, service-account keys, or
file-keyring passwords.

### Workspace service accounts

Workspace admins can configure domain-wide delegation and then store a
service-account key for the user to impersonate:

```bash
gog auth service-account set user@company.com --key ~/Downloads/service-account.json
gog --account user@company.com auth status
```

Service accounts are mainly useful for Workspace Admin, Groups, Keep, and
domain-wide automation. They do not replace normal OAuth for consumer Gmail
accounts.

## Services

Docs: [Command index](docs/commands/README.md),
[Workspace Admin](docs/workspace-admin.md),
[`gog auth services`](docs/commands/gog-auth-services.md).

Common user services:

- Gmail, Calendar, Drive, Docs, Sheets, Slides, Forms, Meet, Zoom, Apps Script
- Analytics and Search Console
- Contacts, People, Tasks, Classroom
- Chat for Workspace accounts
- Backup and local utility commands

Workspace/admin services:

- Admin Directory
- Cloud Identity Groups
- Keep with domain-wide delegation

Admin Directory includes Workspace user creation from the CLI:

```bash
gog --account admin@example.com admin users create ada@example.com \
  --first-name Ada \
  --last-name Lovelace \
  --password 'TempPass123!' \
  --change-password \
  --ou /Engineering
```

Omit `--password` to generate a temporary password. See
[docs/workspace-admin.md](docs/workspace-admin.md) for service-account setup,
user cleanup, recovery fields, organizational units, and group examples.

Workspace organizational units are covered too:

```bash
gog --account admin@example.com admin orgunits list --type all
gog --account admin@example.com admin orgunits create Engineering --parent /
```

Generated service scope table:

<!-- auth-services:start -->
| Service | User | APIs | Scopes | Notes |
| --- | --- | --- | --- | --- |
| gmail | yes | Gmail API | `https://www.googleapis.com/auth/gmail.modify`<br>`https://www.googleapis.com/auth/gmail.settings.basic`<br>`https://www.googleapis.com/auth/gmail.settings.sharing` |  |
| calendar | yes | Calendar API | `https://www.googleapis.com/auth/calendar` |  |
| chat | yes | Chat API | `https://www.googleapis.com/auth/chat.spaces`<br>`https://www.googleapis.com/auth/chat.messages`<br>`https://www.googleapis.com/auth/chat.memberships`<br>`https://www.googleapis.com/auth/chat.users.readstate.readonly`<br>`https://www.googleapis.com/auth/chat.messages.reactions.create`<br>`https://www.googleapis.com/auth/chat.messages.reactions.readonly` |  |
| classroom | yes | Classroom API | `https://www.googleapis.com/auth/classroom.courses`<br>`https://www.googleapis.com/auth/classroom.rosters`<br>`https://www.googleapis.com/auth/classroom.coursework.students`<br>`https://www.googleapis.com/auth/classroom.coursework.me`<br>`https://www.googleapis.com/auth/classroom.courseworkmaterials`<br>`https://www.googleapis.com/auth/classroom.announcements`<br>`https://www.googleapis.com/auth/classroom.topics`<br>`https://www.googleapis.com/auth/classroom.guardianlinks.students`<br>`https://www.googleapis.com/auth/classroom.profile.emails`<br>`https://www.googleapis.com/auth/classroom.profile.photos` |  |
| drive | yes | Drive API | `https://www.googleapis.com/auth/drive` |  |
| driveactivity | yes | Drive Activity API | `https://www.googleapis.com/auth/drive.activity.readonly` | Read-only audit/activity scope; authorize with --services driveactivity |
| drivelabels | yes | Drive Labels API | `https://www.googleapis.com/auth/drive.labels.readonly` | Read-only Drive label schema; authorize with --services drivelabels |
| docs | yes | Docs API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/documents` | Export/copy/create via Drive |
| slides | yes | Slides API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/presentations` | Create/edit presentations |
| contacts | yes | People API | `https://www.googleapis.com/auth/contacts`<br>`https://www.googleapis.com/auth/contacts.other.readonly`<br>`https://www.googleapis.com/auth/directory.readonly` | Contacts + other contacts + directory |
| tasks | yes | Tasks API | `https://www.googleapis.com/auth/tasks` |  |
| sheets | yes | Sheets API, Drive API | `https://www.googleapis.com/auth/drive`<br>`https://www.googleapis.com/auth/spreadsheets` | Export via Drive |
| people | yes | People API | `profile` | OIDC profile scope |
| forms | yes | Forms API | `https://www.googleapis.com/auth/forms.body`<br>`https://www.googleapis.com/auth/forms.responses.readonly` |  |
| sites | yes | Drive API | `https://www.googleapis.com/auth/drive` | New Google Sites are exposed as Drive files |
| meet | yes | Meet REST API | `https://www.googleapis.com/auth/meetings.space.created`<br>`https://www.googleapis.com/auth/meetings.space.readonly`<br>`https://www.googleapis.com/auth/meetings.space.settings` |  |
| appscript | yes | Apps Script API | `https://www.googleapis.com/auth/script.projects`<br>`https://www.googleapis.com/auth/script.deployments`<br>`https://www.googleapis.com/auth/script.processes` |  |
| analytics | yes | Analytics Admin API, Analytics Data API | `https://www.googleapis.com/auth/analytics.readonly` | GA4 account summaries + reporting |
| searchconsole | yes | Search Console API | `https://www.googleapis.com/auth/webmasters` | Search Analytics + sitemap management |
| ads | yes | Google Ads API | `https://www.googleapis.com/auth/adwords` | OAuth scope only |
| groups | no | Cloud Identity API | `https://www.googleapis.com/auth/cloud-identity.groups.readonly` | Workspace only |
| keep | no | Keep API | `https://www.googleapis.com/auth/keep` | Workspace only; service account (domain-wide delegation) |
| admin | no | Admin SDK Directory API | `https://www.googleapis.com/auth/admin.directory.user`<br>`https://www.googleapis.com/auth/admin.directory.group`<br>`https://www.googleapis.com/auth/admin.directory.group.member` | Workspace only; service account with domain-wide delegation required |
| youtube | yes | YouTube Data API v3 | `https://www.googleapis.com/auth/youtube.readonly` | Most read operations also work with API key only (config youtube_api_key or GOG_YOUTUBE_API_KEY) |
| photos | yes | Photos Library API | `https://www.googleapis.com/auth/photoslibrary.readonly.appcreateddata` | Read-only app-created media only after Google Photos Library API scope changes |
<!-- auth-services:end -->

Regenerate the table with:

```bash
go run scripts/gen-auth-services-md.go
```

## Documentation

- [Overview](docs/index.md) — rendered at <https://gogcli.sh/>
- [Install](docs/install.md) — <https://gogcli.sh/install.html>
- [Quickstart](docs/quickstart.md) — <https://gogcli.sh/quickstart.html>
- [Command index](docs/commands/README.md) — <https://gogcli.sh/commands/>
- [Gmail workflows](docs/gmail-workflows.md) — <https://gogcli.sh/gmail-workflows.html>
- [Gmail watch](docs/watch.md) — <https://gogcli.sh/watch.html>
- [Drive audits](docs/drive-audits.md) — <https://gogcli.sh/drive-audits.html>
- [Docs editing](docs/docs-editing.md) — <https://gogcli.sh/docs-editing.html>
- [Sheets tables](docs/sheets-tables.md) and [Sheets formatting](docs/sheets-formatting.md)
- [Safety profiles](docs/safety-profiles.md) — command guards and baked safe binaries
- [Auth clients](docs/auth-clients.md) — OAuth clients, account mapping, and service accounts
- [Workspace Admin](docs/workspace-admin.md) — Workspace user, org unit, and group administration
- [Backup](docs/backup.md) — encrypted Google account backups
- [CHANGELOG.md](CHANGELOG.md): release notes

Every command also has help built in:

```bash
gog --help
gog gmail --help
gog drive inventory --help
gog schema --json
```

## Development

```bash
make tools
make build
make fmt
make lint
make test
make ci
```

Generated command docs:

```bash
make docs-commands
make docs-site
open dist/docs-site/index.html
```

Live Google API smoke tests are opt-in:

```bash
scripts/live-test.sh --fast --account you@gmail.com
GOG_IT_ACCOUNT=you@gmail.com go test -tags=integration ./internal/integration
```

See [docs/RELEASING.md](docs/RELEASING.md) for the release checklist.

## Credits

Inspired by Mario Zechner's original Google CLIs:

- [gmcli](https://github.com/badlogic/gmcli)
- [gccli](https://github.com/badlogic/gccli)
- [gdcli](https://github.com/badlogic/gdcli)

## License

MIT
