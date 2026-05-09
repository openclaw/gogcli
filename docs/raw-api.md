# Raw API Dumps

read_when:
- Using `gog <service> raw` commands for lossless Google API JSON.
- Passing Google API responses into scripts, debuggers, or LLM workflows.
- Reviewing sensitive-field behavior for raw output.

Raw commands return the canonical Google API response shape instead of gog's
normal curated table/JSON output. They are useful when a script needs a field
that gog does not model yet, or when debugging an API object exactly as Google
returns it.

## Commands

- [`gog calendar raw`](commands/gog-calendar-raw.md)
- [`gog contacts raw`](commands/gog-contacts-raw.md)
- [`gog docs raw`](commands/gog-docs-raw.md)
- [`gog drive raw`](commands/gog-drive-raw.md)
- [`gog forms raw`](commands/gog-forms-raw.md)
- [`gog gmail raw`](commands/gog-gmail-raw.md)
- [`gog people raw`](commands/gog-people-raw.md)
- [`gog sheets raw`](commands/gog-sheets-raw.md)
- [`gog slides raw`](commands/gog-slides-raw.md)
- [`gog tasks raw`](commands/gog-tasks-raw.md)

## Examples

```bash
gog drive raw <fileId> --pretty
gog docs raw <docId> --json > doc-api.json
gog gmail raw <messageId> --format metadata --json
gog sheets raw <spreadsheetId> --include-grid-data --json
```

Use service-native field masks when available:

```bash
gog drive raw <fileId> --fields 'id,name,mimeType,owners(emailAddress)' --json
gog contacts raw people/c123 --person-fields names,emailAddresses,phoneNumbers --json
```

## Safety Model

Raw output is intentionally less opinionated than normal gog output. It may
include private document content, contact data, event attendees, Gmail payloads,
or service-specific metadata.

Drive has the highest capability-URL risk. By default, `gog drive raw` redacts
fields such as `thumbnailLink`, `webContentLink`, `exportLinks`, `resourceKey`,
`properties`, `appProperties`, and embedded thumbnail bytes unless the user
explicitly names fields via `--fields`.

Sheets warns when grid data or developer metadata could expose sensitive data,
but keeps output lossless. Docs and Slides may include short-lived image URLs.

For the full sensitive-field review, read [Raw API Sensitive Field Audit](raw-audit.md).

## Automation Tips

- Prefer `--json` for scripts.
- Prefer `--pretty` for humans.
- Use narrow `--fields` or service-specific field masks whenever possible.
- Do not pipe raw output into logs or LLMs unless you are comfortable with the
  object's full Google API payload.
