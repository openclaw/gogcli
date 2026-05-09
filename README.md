# gogcli

`gog` is a script-friendly Google CLI for Gmail, Calendar, Drive, Docs, Sheets,
Slides, Forms, Apps Script, Contacts, Tasks, People, Classroom, Chat, and
Workspace admin flows.

It is built for terminals, shell scripts, CI, and coding agents:

- predictable `--json` and `--plain` output on stdout
- human hints and progress on stderr
- multiple Google accounts and OAuth clients
- OAuth, direct access tokens, ADC, and Workspace service accounts
- runtime command allowlists/denylists and baked safety-profile binaries
- read-only audit/reporting commands for risky surfaces like Drive and Contacts
- generated docs for every command

## Install

### Homebrew

```bash
brew install gogcli
gog --version
```

### Docker

```bash
docker run --rm ghcr.io/steipete/gogcli:latest version
```

Authenticated container runs should use a persistent config volume and the
encrypted file keyring:

```bash
docker volume create gogcli-config

docker run --rm -it \
  -e GOG_KEYRING_BACKEND=file \
  -e GOG_KEYRING_PASSWORD \
  -v gogcli-config:/home/gog/.config/gogcli \
  ghcr.io/steipete/gogcli:latest \
  auth add you@gmail.com --services gmail,calendar,drive
```

### Windows

Download `gogcli_<version>_windows_amd64.zip` or
`gogcli_<version>_windows_arm64.zip` from the
[latest release](https://github.com/steipete/gogcli/releases), extract
`gog.exe`, and put that directory on `PATH`.

### Build from source

```bash
git clone https://github.com/steipete/gogcli.git
cd gogcli
make
./bin/gog --version
```

Source builds require the Go version declared in `go.mod`.

## Quick Start

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
- [Apps Script user setting](https://script.google.com/home/usersettings)

Enable APIs in the same Cloud project that owns your OAuth client. If Google
returns `accessNotConfigured`, enable that API and retry after propagation.

Consumer `gmail.com` accounts work for normal user APIs such as Gmail, Calendar,
Drive, Docs, Sheets, Slides, Forms, Apps Script, Contacts/People, Tasks, and
Classroom. Workspace-only APIs such as Admin Directory, Cloud Identity Groups,
Chat, and Keep/domain-wide-delegation flows require a managed domain.

If your OAuth app is External + Testing, Google refresh tokens for user-data
scopes can expire after 7 days. Publish the personal OAuth app if you want
long-lived refresh tokens.

## Daily Examples

### Gmail

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

```bash
gog calendar events --today
gog calendar create --summary "Review" \
  --from "2026-05-06T10:00:00+02:00" \
  --to "2026-05-06T10:30:00+02:00"
gog calendar update primary <eventId> --with-meet
gog calendar move primary <eventId> team-calendar@example.com
```

### Drive

```bash
# Read-only folder audits.
gog drive tree --parent <folderId> --depth 2
gog drive du --parent <folderId> --max 20 --json
gog drive inventory --parent <folderId> --json

# Ask Drive for non-default fields.
gog drive get <fileId> --fields 'id,name,mimeType,size,owners,emailAddress' --json

# Lossless raw API JSON.
gog drive raw <fileId> --pretty
```

### Contacts

```bash
gog contacts search alice --json
gog contacts export --all --out contacts.vcf

# Preview only: no merge/delete/update call is made.
gog contacts dedupe --json
gog contacts dedupe --match email,phone,name --dry-run
```

### Docs

```bash
gog docs write <docId> --append --markdown --text '## Status'
gog docs format <docId> --match Status --bold --font-size 18
gog docs add-tab <docId> --title "Notes"
gog docs find-replace <docId> old new --tab "Notes" --dry-run
gog docs raw <docId> --pretty
```

### Sheets

```bash
gog sheets get <spreadsheetId> 'Sheet1!A1:D20' --json
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

```bash
gog slides create-from-markdown "Weekly update" --content-file slides.md
gog slides insert-text <presentationId> <objectId> "New text"
gog forms publish <formId>
gog forms responses list <formId> --json
gog forms raw <formId> --pretty
```

### Backup

```bash
gog backup init --repo ~/Backups/gog
gog backup push --services gmail,calendar,contacts,drive
gog backup verify
gog backup export --gmail-format markdown --out ~/Exports/gog
```

See [docs/backup.md](docs/backup.md) before running broad or unattended backup
jobs.

## Output and Automation

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
- `--dry-run`: print intended actions where a command supports planning
- `--no-input`: fail instead of prompting
- `--force`: confirm destructive operations
- `--enable-commands <csv>`: allow only selected command paths
- `--disable-commands <csv>`: block selected command paths
- `--gmail-no-send`: block Gmail send operations

For coding agents or CI, prefer:

```bash
gog --account you@gmail.com \
  --enable-commands gmail.search,gmail.get,drive.ls,docs.cat \
  --gmail-no-send \
  --json \
  gmail search 'newer_than:7d'
```

For stricter agent deployments, build or download a baked safety-profile binary.
See [docs/safety-profiles.md](docs/safety-profiles.md).

## Auth and Accounts

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

Common user services:

- Gmail, Calendar, Drive, Docs, Sheets, Slides, Forms, Apps Script
- Contacts, People, Tasks, Classroom
- Chat for Workspace accounts
- Backup and local utility commands

Workspace/admin services:

- Admin Directory
- Cloud Identity Groups
- Keep with domain-wide delegation

Generated service scope table:

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

Regenerate the table with:

```bash
go run scripts/gen-auth-services-md.go
```

## Documentation

- [docs/index.md](docs/index.md): docs overview (rendered at <https://gogcli.sh/>)
- [docs/quickstart.md](docs/quickstart.md): five-minute setup walkthrough
- [docs/commands/README.md](docs/commands/README.md): generated command index
- [docs/safety-profiles.md](docs/safety-profiles.md): command guards and baked safe binaries
- [docs/auth-clients.md](docs/auth-clients.md): OAuth clients, account mapping, and service accounts
- [docs/sheets-tables.md](docs/sheets-tables.md): structured Sheets tables
- [docs/backup.md](docs/backup.md): encrypted Google account backups
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
