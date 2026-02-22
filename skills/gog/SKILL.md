---
name: gog
description: |
  Google Workspace CLI for AI agents. Use when interacting with Google services:
  - Gmail: search, read, send, reply, draft, manage labels, threads, filters, delegation, vacation, watch
  - Calendar: list events, create/update/delete events, check free/busy, find conflicts, team calendars, focus time, OOO
  - Drive: list, search, upload, download, share files and folders, shared drives, comments
  - Sheets: read, write, append, clear, format, insert rows/cols, notes, create, export
  - Docs: read, export, create, copy, write/update content, find-replace, tabs
  - Slides: export, create, copy, add/replace slides, speaker notes
  - Forms: create forms, list/get responses
  - Apps Script: create/get projects, run functions
  - Contacts: search, list, create, update, delete, directory, other contacts
  - Tasks: list, create, complete, manage task lists, recurring tasks
  - Chat: spaces, messages, threads, DMs (Workspace only)
  - Keep: list, search, get notes (Workspace only, service account required)
  - Groups: list groups, list members (Workspace only)
  - People: directory search, profile lookup, relations
  - Classroom: courses, roster, coursework, submissions, announcements (Workspace for Education)
  - Time: local/UTC time utilities
  Triggers: "gmail", "email", "calendar", "google drive", "spreadsheet", "sheets", "google docs", "slides", "contacts", "google tasks", "google chat", "send email", "search email", "calendar events", "upload to drive", "read spreadsheet", "schedule meeting", "google forms", "apps script"
allowed-tools: Bash(gog:*)
compatibility: Linux, macOS, Windows
metadata:
  author: steipete
license: MIT
---

# gog — Google in your terminal

Fast, script-friendly CLI for Gmail, Calendar, Chat, Classroom, Drive, Docs, Slides, Sheets, Forms, Apps Script, Contacts, Tasks, People, Groups, and Keep. JSON-first output, multiple accounts, and least-privilege auth built in.

## Agent preflight (run this first)

Before running any gog command, always check readiness in this order:

```bash
# 1. Is gog installed?
gog version

# 2. Can we access the keyring? (if file backend, needs GOG_KEYRING_PASSWORD)
gog auth list --no-input 2>&1

# 3. Is the target account authorized?
gog auth list --no-input | grep "target@email.com"
```

**If gog is not installed:** Tell the user to install it (see Installation below).

**If keyring fails** with `no TTY available for keyring file backend password prompt; set GOG_KEYRING_PASSWORD`:
- The user needs to set `GOG_KEYRING_PASSWORD` in their environment (e.g. in `~/.zshrc` or `~/.zshrc.local`)
- Then source it: `source ~/.zshrc` or `source ~/.zshrc.local`

**If the account is not listed:** The user needs to authorize it. This requires human interaction (OAuth browser flow). Tell the user:
1. Run `gog auth add their@email.com --manual` in their terminal
2. Open the printed URL in their browser
3. Sign in and authorize
4. Copy the redirect URL back into the terminal

**If everything works:** Use `--account their@email.com --json --no-input` on all commands.

## Installation

**Homebrew (macOS/Linux):**

```bash
brew install steipete/tap/gogcli
```

**Arch Linux (AUR):**

```bash
yay -S gogcli
```

**Build from source:**

```bash
git clone https://github.com/steipete/gogcli.git
cd gogcli && make
./bin/gog --help
```

**Binary downloads:** [GitHub Releases](https://github.com/steipete/gogcli/releases)

**Verify installation:**

```bash
gog version
```

## Authentication

**IMPORTANT FOR AGENTS:** OAuth requires human interaction — the user must open a URL in their browser and sign in with their Google account. An agent cannot complete this step. If auth is not set up, tell the user what commands to run and guide them through it.

### Before you start: check auth status

Always check if auth is already configured before attempting any gog command:

```bash
gog auth list --no-input 2>&1
```

If this fails with a keyring error like `no TTY available for keyring file backend password prompt; set GOG_KEYRING_PASSWORD`, the user needs to set the `GOG_KEYRING_PASSWORD` env var (or switch to a system keyring backend).

If it returns no accounts, the user needs to set up auth first (see below).

### First-time setup (requires human)

gog requires a Google Cloud project with OAuth credentials. This is a one-time setup:

**Step 1: Create a Google Cloud project and OAuth credentials**

1. Go to https://console.cloud.google.com/projectcreate and create a project (or use an existing one)
2. Enable the APIs you need:
   - Gmail API: https://console.cloud.google.com/apis/api/gmail.googleapis.com
   - Calendar API: https://console.cloud.google.com/apis/api/calendar-json.googleapis.com
   - Drive API: https://console.cloud.google.com/apis/api/drive.googleapis.com
   - Sheets API: https://console.cloud.google.com/apis/api/sheets.googleapis.com
   - People API (Contacts): https://console.cloud.google.com/apis/api/people.googleapis.com
   - Tasks API: https://console.cloud.google.com/apis/api/tasks.googleapis.com
3. Configure OAuth consent screen: https://console.cloud.google.com/auth/branding
4. Create OAuth client: https://console.cloud.google.com/auth/clients
   - Click "Create Client", type: "Desktop app", download the JSON file

**Step 2: Add test users (if app is in "Testing" mode)**

If the OAuth app is not published (still in "Testing" mode), every Google account that wants to authorize **must** be added as a test user — otherwise the OAuth flow will fail with `access_denied`.

- Go to: Google Cloud Console → APIs & Services → OAuth consent screen → **Audience** (in the left sidebar)
- Under "Test users", click "Add users" and add each Google account email
- Reference: https://support.google.com/cloud/answer/15549945

**Step 3: Store credentials and authorize**

```bash
# Store the downloaded OAuth client JSON
gog auth credentials ~/Downloads/client_secret_....json

# Authorize account (opens browser — REQUIRES HUMAN INTERACTION)
gog auth add you@gmail.com

# For headless/WSL/SSH (no browser available), use manual flow:
gog auth add you@gmail.com --services user --manual
# This prints a URL → user opens it in browser → signs in → copies redirect URL back

# Or use the two-step remote flow:
gog auth add you@gmail.com --services user --remote --step 1   # Prints auth URL
# User opens URL, authorizes, copies the redirect URL, then:
gog auth add you@gmail.com --services user --remote --step 2 --auth-url '<redirect-url>'

# Test
gog gmail labels list --account you@gmail.com --no-input
```

### Keyring backends

Tokens are stored in a keyring. The backend depends on your OS:

- **macOS**: Keychain Access (works automatically)
- **Linux with desktop**: Secret Service / GNOME Keyring (works automatically)
- **Linux headless / WSL / CI**: File-based keyring — **requires `GOG_KEYRING_PASSWORD` env var**

```bash
# Check current backend
gog auth keyring

# Switch to file backend (for headless/CI)
gog auth keyring file

# Set password for file backend (add to shell profile for persistence)
export GOG_KEYRING_PASSWORD='your-password-here'
```

If you get `no TTY available for keyring file backend password prompt`, set `GOG_KEYRING_PASSWORD`.

### Multiple accounts

```bash
gog --client work auth credentials ~/Downloads/work-client.json
gog --client work auth add you@company.com

# Account aliases
gog auth alias set work work@company.com
gog gmail search 'is:unread' --account work

# Or set default account via env
export GOG_ACCOUNT=you@gmail.com
```

### Service accounts (Workspace only)

```bash
gog auth service-account set you@yourdomain.com --key ~/Downloads/service-account.json
gog auth status --account you@yourdomain.com
```

### Auth commands

```bash
gog auth status                           # Show current auth state
gog auth list                             # List stored accounts
gog auth list --check                     # Validate stored tokens
gog auth services                         # List available services and scopes
gog auth credentials list                 # List stored OAuth clients
gog auth remove <email>                   # Remove a stored account
gog auth keyring                          # Show/set keyring backend (auto|keychain|file)
gog auth manage                           # Open accounts manager in browser
```

### Environment variables

- `GOG_ACCOUNT` — Default account email or alias
- `GOG_CLIENT` — OAuth client name
- `GOG_KEYRING_BACKEND` — Keyring backend: `auto`, `keychain`, `file`
- `GOG_KEYRING_PASSWORD` — Password for file-based keyring (**required for headless/WSL/CI**)
- `GOG_JSON` — Default JSON output
- `GOG_PLAIN` — Default plain output
- `GOG_COLOR` — Color mode: `auto`, `always`, `never`
- `GOG_TIMEZONE` — Default output timezone (IANA name, `UTC`, or `local`)
- `GOG_ENABLE_COMMANDS` — Comma-separated allowlist of top-level commands (sandboxing)

## Global flags

- `--account=STRING` — Account email or alias
- `--client=STRING` — OAuth client name
- `--json` — JSON output to stdout (best for scripting and agent use)
- `--plain` — Stable TSV output, no colors
- `--force` — Skip confirmations for destructive commands
- `--no-input` — Never prompt; fail instead (required for non-interactive agent use)
- `--enable-commands=STRING` — Restrict available commands (sandboxing)
- `--verbose` — Enable verbose logging

**Agent best practice:** Always use `--json` for machine-readable output and `--no-input` to prevent interactive prompts that block execution.

## Gmail

```bash
# Search threads
gog gmail search 'newer_than:7d' --max 10
gog gmail search 'from:alice@example.com subject:invoice'
gog gmail search 'is:unread label:important' --max 20 --json
gog gmail search 'has:attachment filename:pdf'

# Message-level search (one row per email; --include-body to fetch bodies)
gog gmail messages search 'newer_than:7d' --max 10 --include-body --json

# Read a message
gog gmail get <messageId>
gog gmail get <messageId> --format=metadata --headers="From,To,Subject,Date"
gog gmail get <messageId> --json

# Read a thread (all messages + attachments)
gog gmail thread get <threadId>
gog gmail thread get <threadId> --download                # Download attachments
gog gmail thread get <threadId> --download --out-dir ./attachments
gog gmail thread attachments <threadId>                   # List attachments

# Thread labels
gog gmail thread modify <threadId> --add STARRED --remove INBOX

# Get web URL
gog gmail url <threadId>

# Send email
gog gmail send --to "a@b.com" --subject "Hi" --body "Plain text"
gog gmail send --to "a@b.com" --subject "Hi" --body "Plain" --body-html "<p>HTML</p>"
gog gmail send --to "a@b.com" --subject "Hi" --body-file ./message.txt
gog gmail send --to "a@b.com" --cc "b@b.com" --subject "Report" --attach report.pdf

# Reply
gog gmail send --reply-to-message-id <messageId> --body "Thanks!"
gog gmail send --reply-to-message-id <messageId> --quote --body "My reply"  # Include quoted original
gog gmail send --thread-id <threadId> --reply-all --body "Sounds good."

# Send from alias
gog gmail send --from "alias@example.com" --to "a@b.com" --subject "Hi" --body "Body"

# Email tracking
gog gmail send --to "a@b.com" --subject "Hi" --body-html "<p>Hi</p>" --track
gog gmail track opens <tracking_id>
gog gmail track opens --to recipient@example.com

# Drafts
gog gmail drafts list
gog gmail drafts create --to "a@b.com" --subject "Draft" --body "WIP"
gog gmail drafts get <draftId>
gog gmail drafts update <draftId> --subject "Updated" --body "New body"
gog gmail drafts send <draftId>
gog gmail drafts delete <draftId>

# Labels
gog gmail labels list
gog gmail labels get INBOX --json              # Includes message counts
gog gmail labels create "My Label"
gog gmail labels delete <labelIdOrName>

# Batch operations
gog gmail batch delete <messageId> <messageId>
gog gmail batch modify <messageId> <messageId> --add STARRED --remove INBOX

# Filters
gog gmail filters list
gog gmail filters create --from 'noreply@example.com' --add-label 'Notifications'
gog gmail filters delete <filterId>

# Settings
gog gmail vacation get
gog gmail vacation enable --subject "Out of office" --message "..."
gog gmail vacation disable
gog gmail sendas list
gog gmail autoforward get
gog gmail delegates list

# Watch (Pub/Sub push)
gog gmail watch start --topic projects/<p>/topics/<t> --label INBOX
gog gmail history --since <historyId>
```

## Calendar

```bash
# List events
gog calendar events                                       # Primary calendar, next 10
gog calendar events --today                               # Today only
gog calendar events --tomorrow                            # Tomorrow only
gog calendar events --week                                # This week (Mon-Sun)
gog calendar events --days=7                              # Next 7 days
gog calendar events --from="2026-03-01" --to="2026-03-31" --max=50
gog calendar events --all                                 # All calendars
gog calendar events --query="standup"                     # Search events
gog calendar events <calendarId> --json                   # Specific calendar
gog calendar events --cal Work --cal Personal             # By calendar name

# Get a single event
gog calendar event <calendarId> <eventId>
gog calendar get <calendarId> <eventId> --json

# Search events
gog calendar search "meeting" --today
gog calendar search "meeting" --days 365
gog calendar search "meeting" --from "2026-01-01" --to "2026-01-31" --max 50

# Create an event
gog calendar create primary \
  --summary "Team standup" \
  --from "2026-03-01T09:00:00" --to "2026-03-01T09:30:00" \
  --attendees "alice@example.com,bob@example.com" \
  --with-meet

# All-day event
gog calendar create primary --summary "Offsite" --from "2026-04-01" --to "2026-04-03" --all-day

# Recurring event with reminders
gog calendar create primary \
  --summary "Weekly sync" \
  --from "2026-03-01T10:00:00" --to "2026-03-01T10:30:00" \
  --rrule "RRULE:FREQ=WEEKLY;BYDAY=MO" \
  --reminder "popup:30m" --reminder "email:1d"

# Update an event
gog calendar update <calendarId> <eventId> --summary "New title" --from "..." --to "..."
gog calendar update <calendarId> <eventId> --add-attendee "alice@example.com"

# Delete an event
gog calendar delete <calendarId> <eventId>

# Respond to invitations
gog calendar respond <calendarId> <eventId> --status accepted
gog calendar respond <calendarId> <eventId> --status declined
gog calendar respond <calendarId> <eventId> --status tentative

# Propose a new time
gog calendar propose-time <calendarId> <eventId>

# Availability
gog calendar freebusy --calendars "primary,work@example.com" --from "..." --to "..."
gog calendar conflicts --today

# Special event types
gog calendar focus-time --from "..." --to "..."
gog calendar out-of-office --from "2026-01-20" --to "2026-01-21" --all-day
gog calendar working-location --type office --office-label "HQ" --from "..." --to "..."

# Team calendars (Workspace)
gog calendar team <group-email> --today
gog calendar team <group-email> --week --freebusy

# Other
gog calendar calendars                                    # List all calendars
gog calendar colors                                       # Available color IDs
gog calendar users                                        # Workspace users
gog calendar time --timezone UTC
```

## Drive

```bash
# List files
gog drive ls                                              # Root folder
gog drive ls --parent <folderId> --max=50
gog drive ls --json

# Search
gog drive search "quarterly report" --max 20
gog drive search "mimeType = 'application/pdf'" --raw-query

# File operations
gog drive get <fileId>                                    # Metadata
gog drive download <fileId>                               # Download
gog drive download <fileId> --format pdf --out ./doc.pdf  # Export Google Workspace file
gog drive upload ./report.pdf --parent <folderId>
gog drive upload ./report.pdf --replace <fileId>          # Replace content in-place
gog drive upload ./report.docx --convert                  # Convert to Google format
gog drive copy <fileId> "Copy Name"
gog drive rename <fileId> "New Name"
gog drive move <fileId> --parent <newFolderId>
gog drive delete <fileId>                                 # Move to trash
gog drive delete <fileId> --permanent                     # Permanent delete
gog drive mkdir "New Folder" --parent <folderId>

# Sharing
gog drive share <fileId> --to user --email user@example.com --role writer
gog drive permissions <fileId>
gog drive unshare <fileId> --permission-id <permissionId>

# Shared drives
gog drive drives --max 100
gog drive ls --parent <sharedDriveId>

# URLs
gog drive url <fileId>
```

## Sheets

```bash
# Read
gog sheets get <spreadsheetId> "Sheet1!A1:D10"
gog sheets get <spreadsheetId> "Sheet1!A:Z" --json
gog sheets get <spreadsheetId> "Sheet1!A1:B5" --render=FORMULA
gog sheets metadata <spreadsheetId>                       # Sheet names, tabs
gog sheets notes <spreadsheetId> "Sheet1!A1:B10"          # Cell notes

# Write (pipe-separated cells, comma-separated rows)
gog sheets update <spreadsheetId> "Sheet1!A1:B2" "Name|Email" "Alice|alice@example.com"

# Write with JSON values
gog sheets update <spreadsheetId> "Sheet1!A1:B2" \
  --values-json '[["Name","Email"],["Alice","alice@example.com"]]'

# Copy data validation from existing rows
gog sheets update <spreadsheetId> "Sheet1!A1:C1" "new|row|data" \
  --copy-validation-from "Sheet1!A2:C2"

# Append rows
gog sheets append <spreadsheetId> "Sheet1!A:B" \
  --values-json '[["Bob","bob@example.com"]]'

# Clear a range
gog sheets clear <spreadsheetId> "Sheet1!A1:Z100"

# Format cells
gog sheets format <spreadsheetId> "Sheet1!A1:D1" \
  --format-json '{"textFormat":{"bold":true}}' \
  --format-fields 'userEnteredFormat.textFormat.bold'

# Insert rows/columns
gog sheets insert <spreadsheetId> "Sheet1" rows 2 --count 3
gog sheets insert <spreadsheetId> "Sheet1" cols 3 --after

# Create and export
gog sheets create "New Spreadsheet" --sheets "Sheet1,Sheet2"
gog sheets copy <spreadsheetId> "Backup Copy"
gog sheets export <spreadsheetId> --format csv
gog sheets export <spreadsheetId> --format xlsx
gog sheets export <spreadsheetId> --format pdf --out ./sheet.pdf
```

## Docs

```bash
gog docs cat <docId>                                      # Print as plain text
gog docs cat <docId> --max-bytes 10000
gog docs cat <docId> --tab "Notes"                        # Specific tab
gog docs cat <docId> --all-tabs
gog docs info <docId>                                     # Metadata
gog docs list-tabs <docId>                                # List document tabs
gog docs export <docId> --format pdf --out ./doc.pdf
gog docs export <docId> --format docx --out ./doc.docx
gog docs export <docId> --format txt --out ./doc.txt
gog docs create "New Document"
gog docs create "New Document" --file ./doc.md            # Import markdown
gog docs copy <docId> "Copy of Doc"
gog docs write <docId> --replace --markdown --file ./doc.md
gog docs update <docId> --format markdown --content-file ./doc.md
gog docs find-replace <docId> "old text" "new text"
```

## Slides

```bash
gog slides info <presentationId>
gog slides export <presentationId> --format pdf --out ./deck.pdf
gog slides export <presentationId> --format pptx --out ./deck.pptx
gog slides create "New Presentation"
gog slides create-from-markdown "My Deck" --content-file ./slides.md
gog slides copy <presentationId> "Copy of Deck"
gog slides list-slides <presentationId>
gog slides add-slide <presentationId> ./slide.png --notes "Speaker notes"
gog slides update-notes <presentationId> <slideId> --notes "Updated notes"
gog slides replace-slide <presentationId> <slideId> ./new-slide.png
```

## Contacts

```bash
# Search and list
gog contacts search "Alice Smith" --max 50
gog contacts list --max 100
gog contacts get people/<resourceName>
gog contacts get user@example.com                         # Get by email

# Other contacts (people you've interacted with)
gog contacts other list --max 50
gog contacts other search "John"

# Create and update
gog contacts create --given "John" --family "Doe" --email "john@example.com" --phone "+1234567890"
gog contacts update people/<resourceName> --given "Jane" --email "jane@example.com" --birthday "1990-05-12"
gog contacts delete people/<resourceName>

# Workspace directory
gog contacts directory list --max 50
gog contacts directory search "Jane"
```

## Tasks

```bash
# Task lists
gog tasks lists
gog tasks lists create "My Tasks"

# Tasks
gog tasks list <tasklistId> --max 50
gog tasks get <tasklistId> <taskId>
gog tasks add <tasklistId> --title "Task title" --due 2026-03-01
gog tasks add <tasklistId> --title "Weekly sync" --due 2026-02-01 --repeat weekly --repeat-count 4
gog tasks update <tasklistId> <taskId> --title "New title"
gog tasks done <tasklistId> <taskId>
gog tasks undo <tasklistId> <taskId>
gog tasks delete <tasklistId> <taskId>
gog tasks clear <tasklistId>                              # Clear completed tasks
```

## Forms

```bash
gog forms get <formId>
gog forms create --title "Weekly Check-in" --description "Friday async update"
gog forms responses list <formId> --max 20
gog forms responses get <formId> <responseId>
```

## Apps Script

```bash
gog appscript get <scriptId>
gog appscript content <scriptId>
gog appscript create --title "Automation Helpers"
gog appscript create --title "Bound Script" --parent-id <driveFileId>
gog appscript run <scriptId> myFunction --params '["arg1", 123, true]'
```

## Chat (Workspace only)

```bash
gog chat spaces list
gog chat spaces find "Engineering"
gog chat spaces create "Engineering" --member alice@company.com --member bob@company.com
gog chat messages list spaces/<spaceId> --max 5
gog chat messages list spaces/<spaceId> --thread <threadId>
gog chat messages list spaces/<spaceId> --unread
gog chat messages send spaces/<spaceId> --text "Build complete!"
gog chat threads list spaces/<spaceId>
gog chat dm space user@company.com
gog chat dm send user@company.com --text "ping"
```

## Keep (Workspace only, service account required)

```bash
gog keep list
gog keep get <noteId>
gog keep search "shopping list"
gog keep attachment <attachmentName> --out ./attachment.bin
```

## Groups (Workspace only)

```bash
gog groups list
gog groups members engineering@company.com
```

## People

```bash
gog people me                                             # Your profile
gog people get people/<userId>
gog people search "Alice" --max 5                         # Workspace directory
gog people relations                                      # Your relations
gog people relations people/<userId> --type manager
```

## Classroom (Workspace for Education)

```bash
gog classroom courses list
gog classroom courses list --role teacher
gog classroom courses get <courseId>
gog classroom courses create --name "Math 101"
gog classroom roster <courseId>
gog classroom students add <courseId> <userId>
gog classroom coursework list <courseId>
gog classroom coursework create <courseId> --title "Homework 1" --type ASSIGNMENT --state PUBLISHED
gog classroom submissions list <courseId> <courseworkId>
gog classroom submissions grade <courseId> <courseworkId> <submissionId> --grade 85
gog classroom announcements list <courseId>
gog classroom announcements create <courseId> --text "Welcome!"
gog classroom topics list <courseId>
gog classroom invitations list
gog classroom guardians list <studentId>
```

## Time

```bash
gog time now
gog time now --timezone UTC
```

## Configuration

```bash
gog config path                                           # Show config file location
gog config list                                           # Show all config values
gog config keys                                           # Available config keys
gog config get default_timezone
gog config set default_timezone UTC
gog config unset default_timezone
```

Config file locations:
- macOS: `~/Library/Application Support/gogcli/config.json`
- Linux: `~/.config/gogcli/config.json`
- Windows: `%AppData%\gogcli\config.json`

### Command allowlist (sandboxing for agents)

```bash
# Only allow calendar + tasks commands
gog --enable-commands calendar,tasks calendar events --today

# Or via env
export GOG_ENABLE_COMMANDS=calendar,tasks
```

## Tips for agents

1. **Always use `--json`** for structured, parseable output
2. **Always use `--no-input`** to prevent interactive prompts that block execution
3. **Use `--force`** for destructive operations (delete, clear) to skip confirmation prompts
4. **Use `--account=email`** when multiple accounts are configured
5. **Gmail search syntax** supports: `from:`, `to:`, `subject:`, `has:attachment`, `newer_than:`, `older_than:`, `is:unread`, `label:`, `filename:`, etc.
6. **Sheets values-json** accepts a 2D JSON array: `[["row1col1","row1col2"],["row2col1","row2col2"]]`
7. **Calendar times** accept RFC3339, dates (`2026-03-01`), or relative values (`today`, `tomorrow`, `monday`)
8. **Drive folder IDs** can be found via `gog drive ls` or from Google Drive URLs
9. **Pipe-delimited values** for sheets: cells separated by `|`, rows separated by spaces
10. **Sandboxing**: use `GOG_ENABLE_COMMANDS` to restrict what an agent can access
