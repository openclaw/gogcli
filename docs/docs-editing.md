# Google Docs Editing

read_when:
- Editing Google Docs content, tabs, formatting, comments, or raw Docs output.
- Reviewing Docs write, format, find-replace, or tab commands.

Docs commands cover document creation, export, content writes, find/replace,
comments, tabs, formatting, and raw API inspection.

## Write Markdown

Append Markdown and convert it to Google Docs formatting:

```bash
gog docs write <docId> --append --markdown --text '## Status'
```

Replace the document body with Markdown from a file:

```bash
gog docs write <docId> --replace --markdown --content-file README.md
```

Command pages:

- [`gog docs write`](commands/gog-docs-write.md)
- [`gog docs export`](commands/gog-docs-export.md)
- [`gog docs cat`](commands/gog-docs-cat.md)

## Format Text

Apply text or paragraph formatting:

```bash
gog docs format <docId> --match Status --bold --font-size 18
gog docs format <docId> --match "Action item" --text-color '#b00020'
gog docs format <docId> --match Heading --alignment center --line-spacing 120
```

Use `--match-all` when every occurrence should be formatted.

Command page:

- [`gog docs format`](commands/gog-docs-format.md)

## Tabs

Manage Google Docs tabs:

```bash
gog docs list-tabs <docId>
gog docs add-tab <docId> --title "Notes"
gog docs rename-tab <docId> <tabId> "Archive"
gog docs delete-tab <docId> <tabId> --force
```

Tab-aware commands accept `--tab` by title or ID:

```bash
gog docs write <docId> --append --tab "Notes" --text "Follow-up"
gog docs find-replace <docId> old new --tab "Notes" --dry-run
```

Command pages:

- [`gog docs list-tabs`](commands/gog-docs-list-tabs.md)
- [`gog docs add-tab`](commands/gog-docs-add-tab.md)
- [`gog docs rename-tab`](commands/gog-docs-rename-tab.md)
- [`gog docs delete-tab`](commands/gog-docs-delete-tab.md)

## Find and Replace

```bash
gog docs find-replace <docId> old new --dry-run
gog docs find-replace <docId> old '' --first
gog docs find-replace <docId> PLACEHOLDER --content-file replacement.md --format markdown
```

`--dry-run` is read-only and reports match counts. Empty replacement strings are
allowed and delete matches.

Command page:

- [`gog docs find-replace`](commands/gog-docs-find-replace.md)

## Raw Docs Output

Use raw output when a script needs the Google Docs API object:

```bash
gog docs raw <docId> --pretty
```

See [Raw API Dumps](raw-api.md) for lossless-output safety notes.
