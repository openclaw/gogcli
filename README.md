# gog 🧭 — Google Workspace from the terminal

![gogcli banner](docs/assets/readme-banner.jpg)

[![CI](https://img.shields.io/github/actions/workflow/status/openclaw/gogcli/ci.yml?branch=main&style=flat-square&label=ci)](https://github.com/openclaw/gogcli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/openclaw/gogcli?style=flat-square)](https://github.com/openclaw/gogcli/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/openclaw/gogcli?style=flat-square)](https://go.dev/)
[![License](https://img.shields.io/github/license/openclaw/gogcli?style=flat-square)](LICENSE)
[![Homebrew](https://img.shields.io/badge/Homebrew-openclaw%2Ftap-fbb040?style=flat-square&logo=homebrew)](https://github.com/openclaw/homebrew-tap)

`gog` is one command-line client for Gmail, Calendar, Drive, Docs, Sheets, and
the wider Google Workspace surface. It is built for people, scripts, CI, and
agents that need explicit account routing, machine-readable output, and safety
controls.

```bash
gog --readonly gmail search 'is:unread newer_than:7d' --max 10 --json
gog --readonly calendar events --today --json
gog --readonly drive audit sharing --parent <folderId> --json
```

## Install

Homebrew is the shortest path on macOS and Linux:

```bash
brew install openclaw/tap/gogcli
gog --version
```

With Go:

```bash
go install github.com/openclaw/gogcli/cmd/gog@latest
gog --version
```

The module path moved from `github.com/steipete/gogcli` to
`github.com/openclaw/gogcli`. Until the first release tagged after that move is
published, `@latest` still selects an older tag that declares the previous path
and fails; install a specific version from the old path in the meantime:

```bash
go install github.com/steipete/gogcli/cmd/gog@v0.34.2
```

Docker images, Windows archives, raw macOS/Linux binaries, and source builds
are covered in the [install guide](docs/install.md).

## Quick start

Create a Desktop OAuth client in a [Google Cloud project](https://console.cloud.google.com/auth/clients),
download its JSON file, and authorize only the services you need:

```bash
gog auth credentials set ~/Downloads/client_secret_*.json
gog auth add you@gmail.com --services gmail,calendar,drive
export GOG_ACCOUNT=you@gmail.com
gog auth doctor --check
gog gmail search 'newer_than:7d' --max 10
```

The [five-minute quickstart](docs/quickstart.md) covers API enablement, the OAuth
consent screen, weekly-token-expiry avoidance, headless authorization, and
account defaults.

## Work with Google services

Commands follow the resource you are working with. These are the common entry
points; the [examples](docs/examples.md) and generated
[command index](docs/commands/README.md) cover the full surface.

| Work | Start with |
| --- | --- |
| Mail and calendars | `gog gmail search`, `gog calendar events` |
| Files and sharing | `gog drive ls`, `gog drive audit sharing` |
| Docs, Sheets, Slides, and Forms | `gog docs`, `gog sheets`, `gog slides`, `gog forms` |
| Contacts and tasks | `gog contacts`, `gog tasks` |
| Meetings and chat | `gog meet`, `gog chat`, `gog zoom` |
| Analytics and publishing | `gog analytics`, `gog searchconsole`, `gog youtube` |
| Workspace administration | `gog admin`, `gog groups`, `gog keep` |
| Discovery API fallback | `gog api describe`, `gog api call` |

Consumer Google accounts work with user-facing APIs. Admin Directory, Cloud
Identity Groups, Chat, Keep, and domain-wide delegation require a managed
Google Workspace domain. The [Workspace Admin guide](docs/workspace-admin.md)
explains that setup.

## Automate safely

`--json` emits structured output and `--plain` emits stable TSV. Prompts,
progress, and warnings go to stderr. `--no-input`, `--readonly`, exact command
allowlists, Gmail no-send policy, dry-run plans, and untrusted-content wrapping
let the caller define a narrower execution boundary.

```bash
gog --account you@gmail.com \
  --enable-commands-exact gmail.search,gmail.get \
  --gmail-no-send --readonly --no-input --wrap-untrusted --json \
  gmail search 'newer_than:7d'
```

See [Automation](docs/automation.md) for output and exit-code contracts, and
[Safety Profiles](docs/safety-profiles.md) for binaries with command policy and
locked flag values baked in at build time.

## Accounts and authentication

One installation can route among multiple Google accounts, named OAuth client
projects, direct access tokens, Application Default Credentials, and Workspace
service accounts. Tokens use the platform keyring by default; headless systems
can use the encrypted file backend.

```bash
gog auth list --check
gog auth alias set work you@company.com
gog --account work gmail search 'is:unread'
```

See [OAuth clients](docs/auth-clients.md) for client selection and service
accounts, and [Paths and State](docs/paths.md) for `GOG_HOME`, XDG paths, and
keyring storage.

## Discover the contract

The running binary generates its command schema, reference pages, and agent
skills from the same command tree:

```bash
gog schema --json
gog schema gmail search --json
gog help drive inventory
```

`gog mcp` exposes a typed stdio MCP server without a generic shell or command
bridge. It is read-only by default; writes require explicit command and tool
authorization. See the [MCP guide](docs/mcp.md).

## Supported OAuth services

The generated table below records the user OAuth and Workspace service-account
surface. `gog auth services` reports the same information from the installed
binary.

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
| photospicker | no | Photos Picker API | `https://www.googleapis.com/auth/photospicker.mediaitems.readonly` | Consumer OAuth; explicit opt-in with --services photospicker; selected media only |
<!-- auth-services:end -->

## Documentation

- [Overview and guides](https://gogcli.sh/)
- [Examples](docs/examples.md)
- [Command index](docs/commands/README.md)
- [Gmail workflows](docs/gmail-workflows.md)
- [Drive audits](docs/drive-audits.md)
- [Docs, Sheets, and Slides guides](docs/index.md#pick-your-path)
- [Changelog](CHANGELOG.md)

`gog` is open source and is not affiliated with Google.

## Credits

Inspired by Mario Zechner's [gmcli](https://github.com/badlogic/gmcli),
[gccli](https://github.com/badlogic/gccli), and
[gdcli](https://github.com/badlogic/gdcli).

## Development

The project requires the Go version declared in `go.mod`.

```bash
make build
make test
make ci
```

See [live testing](docs/live-testing.md) for opt-in Google API smoke tests and
[releasing](docs/RELEASING.md) for the maintainer workflow.

## License

[MIT](LICENSE)
