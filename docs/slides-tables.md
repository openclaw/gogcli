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

Use `--dry-run --json` to inspect the exact Slides `batchUpdate` request without
contacting Google. Use `slides read-slide --detail --json` to verify the table
dimensions, cell coordinates, and text returned by the provider.
