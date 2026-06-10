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

Promote an existing paragraph to a heading or title style with
`--heading-level N` (1..6 shortcut) or `--named-style NAME` (full enum:
`NORMAL_TEXT`, `TITLE`, `SUBTITLE`, `HEADING_1`..`HEADING_6`,
case-insensitive). Both set `paragraphStyle.namedStyleType` on the same
update so they compose with `--alignment` and `--line-spacing`:

```bash
gog docs format <docId> --match "Status" --heading-level 2
gog docs format <docId> --match "Overview" --named-style title --alignment center
```

Use `--match-all` when every occurrence should be formatted.

Command page:

- [`gog docs format`](commands/gog-docs-format.md)

## Page Breaks

Markdown has no native page-break construct, so multi-page deliverables need a
direct Docs API call. Insert a page break at a specific index or append one at
end-of-doc:

```bash
gog docs insert-page-break <docId> --at-end
gog docs insert-page-break <docId> --index 250 --tab "Notes"
```

`--index` and `--at-end` are mutually exclusive; omit both to default to
end-of-doc. Aliases: `page-break`, `pb`.

Command page:

- [`gog docs insert-page-break`](commands/gog-docs-insert-page-break.md)

## Page Layout

Set an existing document to pageless or paged mode:

```bash
gog docs page-layout <docId> --layout pageless
gog docs page-layout <docId> --layout pages
```

Use explicit page size and margin flags when the output width matters:

```bash
gog docs page-layout <docId> --page-width 960
gog docs page-layout <docId> --layout pages --page-width 8.5in --page-height 11in \
  --margin-left 0.5in --margin-right 0.5in
gog docs write <docId> --replace --markdown --file report.md --pageless --page-width 960
```

Lengths default to points and also accept `pt`, `in`, `cm`, or `mm`.
`docs page-layout` preserves the current page mode when only size or margin
flags are supplied; pass `--layout` when you also want to toggle pageless/pages.
`--pageless` preserves Google Docs' existing width unless `--page-width` is set
explicitly.

Command page:

- [`gog docs page-layout`](commands/gog-docs-page-layout.md)

## Tables

Insert a native Google Docs table directly via the Docs API, bypassing the
Markdown writer:

```bash
gog docs insert-table <docId> --rows 3 --cols 2 --at-end
gog docs insert-table <docId> --rows 2 --cols 2 --index 1 \
  --values-json '[["A","B"],["C","D"]]'
```

`--values-json` takes a JSON 2D string array whose dimensions must match
`--rows`x`--cols`; omit it to insert an empty table structure. Use `--at-end`
to append at the end of the document (or the selected `--tab`), or `--index N`
to insert at a specific document index. Prefer this primitive when you want a
guaranteed native table rather than relying on the Markdown writer's table
rendering (see `gog docs write --markdown`).

Update one existing table cell without round-tripping the surrounding document:

```bash
gog docs cell-update <docId> --table-index 1 --row 2 --col 3 \
  --content "**Ready**" --format markdown
```

Coordinates are 1-based. `--tab` targets a specific tab, and `--append` inserts
at the end of the cell instead of replacing its current content.

Set or reset native table column widths after inserting or importing tables:

```bash
gog docs table-column-width <docId> --table-index 1 --col 1 --width 120
gog docs table-column-width <docId> --table-index 1 --evenly-distributed
```

`--width` uses points and requires `--col`. `--evenly-distributed` resets one
column when `--col` is supplied, or all columns when it is omitted.

Command page:

- [`gog docs insert-table`](commands/gog-docs-insert-table.md)
- [`gog docs cell-update`](commands/gog-docs-cell-update.md)
- [`gog docs table-column-width`](commands/gog-docs-table-column-width.md)

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

Re-render an entire tab from a markdown source-of-truth file with
`--replace --markdown --tab`:

```bash
gog docs write <docId> --replace --markdown --tab "Gold list" --file gold.md
```

Drive's markdown converter is whole-document-only, so this path wipes the
targeted tab's content via `DeleteContentRange` and re-renders the markdown
locally through the Docs API. Other tabs are untouched.

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

`--dry-run` is fully offline and reports the intended replacement without
opening the document. Empty replacement strings are allowed and delete matches.

Command page:

- [`gog docs find-replace`](commands/gog-docs-find-replace.md)

## Raw Docs Output

Use raw output when a script needs the Google Docs API object:

```bash
gog docs raw <docId> --pretty
gog docs raw <docId> --tab "Notes" --pretty
gog docs raw <docId> --all-tabs --json
```

`--tab` returns one tab in the same top-level Document shape used by the
default response. `--all-tabs` returns the canonical recursive `tabs` tree.

See [Raw API Dumps](raw-api.md) for lossless-output safety notes.
