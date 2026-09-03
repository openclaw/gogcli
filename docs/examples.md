# Examples

This page collects task-oriented `gog` examples by Google service. Every
command also has generated usage and flag documentation in the
[command index](commands/README.md).

## Gmail

See [Gmail workflows](gmail-workflows.md), [Gmail watch](watch.md), and
[email tracking](email-tracking.md).

```bash
# Search mail and get sanitized message content for agents or scripts.
gog gmail search 'from:boss newer_than:30d' --json
gog gmail get <messageId> --sanitize-content --json

# Export Gmail filters in the format the Gmail web UI can import.
gog gmail settings filters export --out filters.xml

# Block send operations during automation.
gog --gmail-no-send gmail drafts create --to you@example.com --subject test
```

Permanent deletion with `gog gmail batch delete` requires the broader
`https://mail.google.com/` OAuth scope. Prefer `gog gmail trash` unless
permanent deletion is intentional.

## Calendar

See the [`gog calendar`](commands/gog-calendar.md) reference and
[Zoom setup](zoom-auth-setup.md).

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
gog calendar update primary <eventId> \
  --attachment 'https://drive.google.com/open?id=<fileId>'
gog calendar update primary <eventId> --attachment ''
gog zoom auth setup
gog calendar create primary --summary "Client sync" \
  --from "2026-05-06T11:00:00+02:00" \
  --to "2026-05-06T11:30:00+02:00" \
  --with-zoom
gog calendar move primary <eventId> team-calendar@example.com
gog calendar create-calendar "Project calendar" --timezone Europe/London
gog calendar delete-calendar <calendarId> --force
gog calendar subscribe en.uk#holiday@group.v.calendar.google.com
gog calendar unsubscribe en.uk#holiday@group.v.calendar.google.com --force
```

Repeated `--attachment` values replace all attachments; an empty value clears
them. Google Calendar appointment schedules are not exposed by the Calendar
API, so `gog` cannot list or manage them.

## Drive

See [Drive audits](drive-audits.md), [polling](polling.md), and
[raw API dumps](raw-api.md).

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

# Recursively push local contents without deleting remote-only files.
# Listing errors, repeated page tokens, and page-limit failures stop preflight.
# Remote writes begin only after the complete recursive plan succeeds.
gog drive sync push ./backup --parent <folderId> --dry-run --json
gog drive sync push ./backup --parent <folderId>

# Ask Drive for non-default fields.
gog drive get <fileId> --fields 'id,name,mimeType,size,owners,emailAddress' --json

# Track changes and audit activity.
gog drive changes start-token
gog drive changes list --token <token> --json
gog drive changes poll --state-file ~/.local/state/gog/drive-changes.json --json
gog drive changes serve --state-file ~/.local/state/gog/drive-serve.json \
  --channel-token-file ~/.config/gog/drive-channel-token --auto-renew \
  --webhook-url https://example.com/drive-changes
gog drive revisions list <fileId> --all --json
gog drive revisions get <fileId> <revisionId> --json
gog drive activity query --file <fileId> --actions edit,share \
  --from 2026-01-01T00:00:00Z --json

# Lossless raw API JSON.
gog drive raw <fileId> --pretty
```

Drive Labels requires a Google Workspace customer. The Drive API exposes
revision metadata and provider export links; for native Docs Editors files, it
does not expose complete editor history or historical bodies.

## Maps and Photos

See the [`gog maps`](commands/gog-maps.md) reference and
[Photos Picker workflows](photos-picker.md).

```bash
gog maps places search "Elysian Coffee Vancouver" --json
gog maps places details <placeId> --json
gog maps directions --origin "Vancouver, BC" --destination "Seattle, WA" --json
gog maps distance --origins "Vancouver BC" --destinations "Seattle WA" --json
gog maps geocode "1600 Amphitheatre Parkway, Mountain View, CA" --json
gog maps reverse-geocode --lat=37.422 --lng=-122.084 --json

gog photos list --json
gog photos search --media-type PHOTO --from 2026-01-01 --to 2026-01-31 --json
gog photos download <mediaItemId> --out photo.jpg

gog auth add you@gmail.com --services photospicker
gog photos picker create --max-items 20 --open --json
gog photos picker wait <sessionId> --json
gog photos picker list <sessionId> --all --json
gog photos picker download <sessionId> <mediaItemId> --out photo.jpg
gog photos picker delete <sessionId>
```

Use comma-separated `maps distance --origins/--destinations` values for
multiple locations. If an address contains commas, use a Place ID or latitude
and longitude to avoid splitting it. Photos Library API access is limited to
app-created media; Photos Picker provides explicit user selection for private
media and requires its separate service authorization.

## Contacts

See [contact deduplication](contacts-dedupe.md) and
[JSON contact updates](contacts-json-update.md).

```bash
gog contacts search alice --json
gog contacts export --all --out contacts.vcf

# Preview by default, then inspect and apply the mutation plan.
gog contacts dedupe --json
gog contacts dedupe --match email,phone,name
gog contacts dedupe --apply --dry-run --json
gog contacts dedupe --apply

# Scope automation to reviewed contact resources.
gog contacts dedupe --resource people/123 --resource people/456 \
  --apply --force --json
```

Contact exports include user-defined group names as vCard categories. Group
names are collected from all pages before output is written. If Google repeats
a group page token or a group page fails, the command returns an error without
writing a partial export.

## Docs

See [Google Docs editing](docs-editing.md), [atomic request batches](docs-batch.md),
[polling](polling.md), and [sed-style edits](sedmat.md).

```bash
gog docs write <docId> --append --markdown --text '## Status'
gog docs format <docId> --match Status --bold --font-size 18
gog docs format <docId> --match "Project site" --link https://example.com
gog docs format <docId> --match "Action item" --bullets --space-below 6
gog docs find-range <docId> "Release status" --json
gog docs insert-page-break <docId> --at-end
gog docs insert-table <docId> --rows 3 --cols 2 --at-end
gog docs named-range create <docId> --name Status --at "Ready"
gog docs insert-image <docId> --url https://example.com/chart.png --at end
gog docs add-tab <docId> --title "Notes"
gog docs tabs add <docId> --title "Notes"
gog docs comments poll <docId> \
  --state-file ~/.local/state/gog/doc-comments.json --json
gog docs find-replace <docId> old new --tab "Notes" --dry-run
gog docs raw <docId> --pretty
```

## Sheets

See [batch updates](sheets-batch-update.md), [tables](sheets-tables.md), and
[formatting](sheets-formatting.md).

```bash
gog sheets get <spreadsheetId> 'Sheet1!A1:D20' --json
gog sheets update <spreadsheetId> 'Sheet1!B13' \
  --values-json @formula.json --fail-on-formula-error --json
gog sheets batch-update <spreadsheetId> --data-json @updates.json --json
gog sheets table list <spreadsheetId>
gog sheets table append <spreadsheetId> Tasks 'Ship README|done'
gog sheets table clear <spreadsheetId> Tasks
gog sheets validation set <spreadsheetId> 'Sheet1!B2:B100' \
  --type ONE_OF_LIST --value Open --value Done
gog sheets links set <spreadsheetId> 'Sheet1!C2' \
  https://example.com "Project"
gog sheets delete-dimension <spreadsheetId> 'Sheet1!3:3' \
  --dimension ROWS --force
gog sheets conditional-format add <spreadsheetId> 'Sheet1!A2:A100' \
  --type text-contains --expr blocked \
  --format-json '{"backgroundColor":{"red":1,"green":0.84,"blue":0.84}}'
gog sheets banding set <spreadsheetId> 'Sheet1!A1:D100'
```

## Slides and Forms

See [Slides from Markdown](slides-markdown.md),
[template replacement](slides-template-replacement.md),
[introspection](slides-introspection.md), [text editing](slides-text-editing.md),
[tables](slides-tables.md), and [slide structure](slides-structure.md).

```bash
gog slides create-from-markdown "Weekly update" --content-file slides.md
gog slides info <presentationId> --json
gog slides read-slide <presentationId> <slideId> --detail --json
gog slides locate <presentationId> "Quarterly revenue" --all --json
gog slides style-text <presentationId> <objectId> --range 0:12 --bold --size 24
gog slides replace-text <presentationId> old new --object <objectId>
gog slides table create <presentationId> <slideId> --rows 2 --cols 3
gog slides insert-text <presentationId> <tableId> "Revenue" \
  --row 0 --col 0 --replace
gog slides table row size <presentationId> <tableId> --row 0 --height 48
gog slides table cell style <presentationId> <tableId> \
  --row 0 --col 0 --bold --fill-color '#3367d6'
gog slides new-slide <presentationId> --layout TITLE_AND_BODY --index 1
gog slides duplicate-slide <presentationId> <slideId> --to-index 2
gog slides move-slide <presentationId> <slideId> --to-index 0
gog slides insert-image <presentationId> <slideId> chart.png \
  --x 24 --y 24 --width 240
gog slides insert-text <presentationId> <objectId> "New text"
gog forms update <formId> --quiz=true
gog forms add-question <formId> --title "What is 2+2?" \
  --type radio -o 1 -o 4 --correct 4 --points 1
gog forms questions add <formId> --title "What is 2+2?" \
  --type radio -o 1 -o 4 --correct 4 --points 1
gog forms publish <formId>
gog forms responses list <formId> --json
gog forms raw <formId> --pretty
```

## YouTube

See [YouTube workflows](youtube.md) and the
[`gog youtube`](commands/gog-youtube.md) reference.

```bash
gog config set youtube_api_key YOUR_API_KEY
gog yt channels list --id UC_x5XG1OV2P6uZZ5FSM9Ttw --json
gog yt videos list --chart mostPopular --region US --max 5
gog yt activities list --mine -a you@gmail.com
gog yt subscriptions list --all -a you@gmail.com
gog yt playlists list --mine -a you@gmail.com
gog yt playlists items list --playlist-id PLAYLIST_ID --all
gog yt videos list --my-rating like -a you@gmail.com
gog yt playlists create --title "Research" -a you@gmail.com
```

API-key reads require YouTube Data API v3. Subscription and playlist mutations
need the `youtube.force-ssl` scope:

```bash
gog auth add you@gmail.com --services youtube \
  --extra-scopes https://www.googleapis.com/auth/youtube.force-ssl \
  --force-consent
```

All YouTube mutations support `--dry-run`. Destructive mutations also require
confirmation or `--force`, and new playlists default to private.

## Analytics and Search Console

```bash
gog analytics accounts --all --json
gog analytics report 123456789 --from 7daysAgo --to today \
  --dimensions date,country --metrics activeUsers,sessions
gog searchconsole sites
gog searchconsole query sc-domain:example.com \
  --from 2026-02-01 --to 2026-02-07 \
  --dimensions query,page --filter query:contains:gog
gog searchconsole sitemaps submit sc-domain:example.com \
  https://example.com/sitemap.xml --force
```

## Backup

Read the [backup guide](backup.md) before running broad or unattended jobs.

```bash
gog backup init --repo ~/Backups/gog
gog backup push --services gmail,calendar,contacts,drive
gog backup verify
gog backup export --gmail-format markdown --out ~/Exports/gog
```
