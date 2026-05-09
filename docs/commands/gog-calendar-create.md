# `gog calendar create`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Create an event

## Usage

```bash
gog calendar (cal) create (add,new) <calendarId> [flags]
```

## Parent

- [gog calendar](gog-calendar.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--all-day` | `bool` |  | All-day event (use date-only in --from/--to) |
| `--attachment` | `[]string` |  | File attachment URL (can be repeated) |
| `--attendees` | `string` |  | Comma-separated attendee emails |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--description` | `string` |  | Description |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `--end-timezone`<br>`--to-timezone` | `string` |  | IANA timezone metadata for --to (e.g., America/New_York) |
| `--event-color` | `string` |  | Event color ID (1-11). Use 'gog calendar colors' to see available colors. |
| `--event-type` | `string` |  | Event type: default, focus-time, out-of-office, working-location |
| `--focus-auto-decline` | `string` |  | Focus Time auto-decline mode: none, all, new |
| `--focus-chat-status` | `string` |  | Focus Time chat status: available, doNotDisturb |
| `--focus-decline-message` | `string` |  | Focus Time decline message |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--from` | `string` |  | Start time (RFC3339) |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `--guests-can-invite` | `*bool` |  | Allow guests to invite others |
| `--guests-can-modify` | `*bool` |  | Allow guests to modify event |
| `--guests-can-see-others` | `*bool` |  | Allow guests to see other guests |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--location` | `string` |  | Location |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--ooo-auto-decline` | `string` |  | Out of Office auto-decline mode: none, all, new |
| `--ooo-decline-message` | `string` |  | Out of Office decline message |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--private-prop` | `[]string` |  | Private extended property (key=value, can be repeated) |
| `--reminder` | `[]string` |  | Custom reminders as method:duration (e.g., popup:30m, email:1d). Can be repeated (max 5). |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--rrule` | `[]string` |  | Recurrence rules (e.g., 'RRULE:FREQ=MONTHLY;BYMONTHDAY=11'). Can be repeated. |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--send-updates` | `string` |  | Notification mode: all, externalOnly, none (default: none) |
| `--shared-prop` | `[]string` |  | Shared extended property (key=value, can be repeated) |
| `--source-title` | `string` |  | Title of the source |
| `--source-url` | `string` |  | URL where event was created/imported from |
| `--start-timezone`<br>`--from-timezone` | `string` |  | IANA timezone metadata for --from (e.g., Europe/Rome) |
| `--summary` | `string` |  | Event summary/title |
| `--to` | `string` |  | End time (RFC3339) |
| `--transparency` | `string` |  | Show as busy (opaque) or free (transparent). Aliases: busy, free |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--visibility` | `string` |  | Event visibility: default, public, private, confidential |
| `--with-meet` | `bool` |  | Create a Google Meet video conference for this event |
| `--working-building-id` | `string` |  | Working location building ID |
| `--working-custom-label` | `string` |  | Working location custom label |
| `--working-desk-id` | `string` |  | Working location desk ID |
| `--working-floor-id` | `string` |  | Working location floor ID |
| `--working-location-type` | `string` |  | Working location type: home, office, custom |
| `--working-office-label` | `string` |  | Working location office name/label |

## See Also

- [gog calendar](gog-calendar.md)
- [Command index](README.md)
