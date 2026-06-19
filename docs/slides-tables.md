# Slides tables

Create a native table on an existing slide:

```bash
gog slides table create <presentationId> <slideId> --rows 3 --cols 4
```

The command prints the table object ID. Pass `--object-id` to choose a stable ID
when a later script needs to reference the table without parsing the response.

Google Slides chooses a new table's initial size and position. The Slides API
ignores size and transform fields on table creation, so this command does not
expose geometry flags that would report a result the provider does not honor.

## Cell text

Target a cell with zero-based `--row` and `--col` indexes:

```bash
gog slides insert-text <presentationId> <tableId> "Revenue" --row 0 --col 0
gog slides insert-text <presentationId> <tableId> '$1.2M' --row 1 --col 0 --replace
```

`--row` and `--col` must be provided together. `--replace` clears and inserts
within the selected cell in one Slides batch update; it does not alter other
cells. `--insertion-index` selects an index within the cell when appending or
inserting without `--replace`.

## Rows, columns, and merged cells

Table structure commands also use zero-based coordinates:

```bash
gog slides table row insert <presentationId> <tableId> --row 1 --below --count 2
gog slides table row delete <presentationId> <tableId> --row 3 --force
gog slides table column insert <presentationId> <tableId> --col 0 --right
gog slides table column delete <presentationId> <tableId> --col 2 --force
gog slides table merge <presentationId> <tableId> --row 0 --col 0 --row-span 1 --col-span 3
gog slides table unmerge <presentationId> <tableId> --row 0 --col 0 --row-span 1 --col-span 3
```

Insertions accept 1-20 rows or columns per request and default to inserting
above or left of the reference cell. The commands fetch current provider
dimensions before mutation, reject ranges outside the table, and submit the
write against the fetched presentation revision.

Row and column deletion require `--force`. Google deletes every row or column
spanned by the reference cell, so an anchor inside a merged cell can remove
multiple dimensions. Deleting the last remaining row or column deletes the
whole table.

Use `--dry-run --json` to inspect the exact Slides `batchUpdate` request without
contacting Google. Use `slides read-slide --detail --json` to verify the table
dimensions, cell coordinates, and text returned by the provider.
