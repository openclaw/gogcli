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

The local Markdown renderer keeps headings adjacent to their following body,
keeps inline replacements in their current paragraph unless the replacement
ends with a newline, resets inherited styles before applying replacement
formatting, and renders HTML `<br>` variants inside table cells. Fenced code
blocks use Roboto Mono with dark-green text and paragraph shading.

Add `--check-orphans` to block the replacement when an open, currently
located comment quote would disappear:

```bash
gog docs write <docId> --replace --markdown --file README.md --check-orphans
```

The preflight uses the same entity and whitespace matching as
`docs comments locate`, skips resolved, unquoted, and already-orphaned
comments, and exits with code 11 before mutation. With `--tab`, only comments
located in the replaced tab are checked. JSON output includes the comments in
`wouldOrphan`; human output is written to stderr.

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

## Ranges, Links, And Anchors

Resolve literal text to Docs API UTF-16 indexes before composing a lower-level
edit:

```bash
gog docs find-range <docId> "Release status" --json
gog docs find-range <docId> "Owner" --all --tab "Plan" --plain
```

Emoji and other non-BMP characters occupy two UTF-16 code units. The returned
indexes are ready for Docs API ranges; do not recalculate them from byte or
rune offsets.

Set or clear a hyperlink on matched text:

```bash
gog docs format <docId> --match "Project site" --link https://example.com
gog docs format <docId> --match "Project site" --no-link
```

`--link` accepts HTTP(S), `mailto:`, bookmark IDs, and same-document heading
slugs. `--link` and `--no-link` are mutually exclusive.

The direct mutators can resolve the same literal anchors:

```bash
gog docs insert <docId> "Prefix: " --at "Release status"
gog docs update <docId> --at "Draft" --text "Ready"
gog docs delete <docId> --at "Remove me"
gog docs insert-person <docId> --email owner@example.com --at "@owner"
gog docs insert-page-break <docId> --at "Appendix"
```

Use `--occurrence N` when an anchor is ambiguous and `--match-case` when case
must be exact. `docs comments locate` applies the same matching rules to a
comment's quoted text and reports its tab plus UTF-16 range.

Command pages:

- [`gog docs find-range`](commands/gog-docs-find-range.md)
- [`gog docs comments locate`](commands/gog-docs-comments-locate.md)

## Discover Content

List document elements before using index- or object-ID-based edit commands:

```bash
gog docs tables list <docId> --json
gog docs images list <docId> --plain
gog docs headings list <docId> --level 2
gog docs paragraphs list <docId> --style NORMAL_TEXT --tab "Notes"
```

All four commands accept `--tab` by title or ID. JSON output includes stable
element indexes and Docs API positions; `--plain` emits headerless TSV for
shell pipelines. Paragraph JSON also reports `isEmpty` plus each text run's
UTF-16 range, text style, and link metadata.

Command pages:

- [`gog docs tables list`](commands/gog-docs-tables-list.md)
- [`gog docs images list`](commands/gog-docs-images-list.md)
- [`gog docs headings list`](commands/gog-docs-headings-list.md)
- [`gog docs paragraphs list`](commands/gog-docs-paragraphs-list.md)

## Named Ranges

Create a durable document anchor from matched text or explicit Docs API
indexes:

```bash
gog docs named-range create <docId> --name ReleaseStatus --at "Ready to ship"
gog docs named-range create <docId> --name ReleaseStatus --start 42 --end 55
```

List, replace, or delete the range by name or ID:

```bash
gog docs named-range list <docId> --json
gog docs named-range replace <docId> ReleaseStatus --text "Released"
gog docs named-range delete <docId> ReleaseStatus
```

Commands are tab-aware. Without `--tab`, they target the default tab and scope
named-range mutations correctly even when the document has additional tabs.
Use `--occurrence` when `--at` matches more than once, and `--match-case` when
case matters.

Command page:

- [`gog docs named-range`](commands/gog-docs-named-range.md)

## Images

Insert a public HTTPS image directly without uploading it to Drive:

```bash
gog docs insert-image <docId> --url https://example.com/chart.png --at end
```

Use `--file` for local PNG, JPEG, or GIF input. Local files are uploaded to
Drive and may require link sharing; `--url` leaves Drive permissions unchanged.
Both modes accept `--tab`, `--width`, and `--height`.

Command page:

- [`gog docs insert-image`](commands/gog-docs-insert-image.md)

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
rendering (see `gog docs write --markdown`). Markdown rendering accepts
one-column tables as well as wider tables.

Update one existing table cell without round-tripping the surrounding document:

```bash
gog docs cell-update <docId> --table-index 1 --row 2 --col 3 \
  --content "**Ready**" --format markdown
gog docs cell-update <docId> --table-index 1 --row 2 --col 3 \
  --content $'- First\n- Second'
```

Coordinates are 1-based. `--tab` targets a specific tab, and `--append` inserts
at the end of the cell instead of replacing its current content. Markdown list
content creates native Google Docs bullets or numbering inside the cell,
including nested levels. Markdown table imports preserve the same nested list
structure inside cells.

Set or reset native table column widths after inserting or importing tables:

```bash
gog docs table-column-width <docId> --table-index 1 --col 1 --width 120
gog docs table-column-width <docId> --table-index 1 --evenly-distributed
```

`--width` uses points and requires `--col`. `--evenly-distributed` resets one
column when `--col` is supplied, or all columns when it is omitted.

Insert or delete rows and columns without using the `docs sed` table syntax:

```bash
gog docs table-row insert <docId> --table 2 --at end
gog docs table-row insert <docId> --table "Status" --at 2 \
  --values-json '["Ready","Owner"]'
gog docs table-row delete <docId> --table -1 --row 3
gog docs table-column insert <docId> --table 1 --at 2
gog docs table-column delete <docId> --table '*' --col -1
```

`--table` accepts a 1-based index, a negative index counted from the end, exact
first-cell text, or `*` for every table. Prefix numeric or syntax-looking header
text with `text:`, for example `--table text:2026` or `--table 'text:*'`.
Header-text matches must be unique. Row and column indexes are 1-based and may
be negative; `--at end` appends. `--values-json` accepts one JSON string array
and is limited to a single table.

Merge a rectangular cell range or unmerge the region containing one cell:

```bash
gog docs table-merge <docId> --table 1 --range 1,1:1,3
gog docs table-unmerge <docId> --table 1 --cell 1,1
```

All direct table mutation commands accept `--tab`, `--dry-run`, `--json`, and
`--plain`. Multi-table mutations are preflighted and preserve descending
document order across Docs API-capped batch updates. Row and column structural
operations reject non-rectangular API table shapes rather than guessing a cell
reference that could broaden a merged-cell mutation.

Command page:

- [`gog docs insert-table`](commands/gog-docs-insert-table.md)
- [`gog docs cell-update`](commands/gog-docs-cell-update.md)
- [`gog docs table-row`](commands/gog-docs-table-row.md)
- [`gog docs table-column`](commands/gog-docs-table-column.md)
- [`gog docs table-merge`](commands/gog-docs-table-merge.md)
- [`gog docs table-unmerge`](commands/gog-docs-table-unmerge.md)
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
