# Sheets Tables

Use `gog sheets table` to manage Google Sheets structured tables. Tables are
different from plain cell ranges: Sheets tracks a table ID, table name, typed
columns, and a bounded table range.

## When To Use

- Use tables when a spreadsheet has structured rows with stable column names.
- Use normal `gog sheets get`, `update`, `append`, and `clear` for plain ranges.
- Use named ranges when you only need a reusable range selector.

## Create A Table

Create requires a spreadsheet ID, a sheet-qualified range, a table name, and
column definitions:

```bash
gog sheets table create "$spreadsheet_id" 'Sheet1!A1:C4' \
  --name Tasks \
  --columns-json '[{"columnName":"Task","columnType":"TEXT"},{"columnName":"Amount","columnType":"DOUBLE"},{"columnName":"Done","columnType":"BOOLEAN"}]'
```

`--columns-json` accepts inline JSON or `@file`. If `columnType` is omitted, it
defaults to `TEXT`.

```json
[
  {"columnName": "Task"},
  {"columnName": "Amount", "columnType": "DOUBLE"},
  {"columnName": "Done", "columnType": "BOOLEAN"}
]
```

The range can be A1 notation with a sheet name, or an existing named range:

```bash
gog sheets table create "$spreadsheet_id" MyNamedRange \
  --name Tasks \
  --columns-json @columns.json
```

## Column Types

`gog` validates table column types before sending the mutation to Google. Use
the Sheets API enum names:

| Use | Instead of |
| --- | --- |
| `DOUBLE` | `NUMBER` |
| `BOOLEAN` | `CHECKBOX` |
| `RATINGS_CHIP` | `RATING` |
| `FILES_CHIP`, `PEOPLE_CHIP`, `FINANCE_CHIP`, or `PLACE_CHIP` | `SMART_CHIP` |

Supported create types are `TEXT`, `DOUBLE`, `CURRENCY`, `PERCENT`, `DATE`,
`TIME`, `DATE_TIME`, `BOOLEAN`, `DROPDOWN`, `FILES_CHIP`, `PEOPLE_CHIP`,
`FINANCE_CHIP`, `PLACE_CHIP`, and `RATINGS_CHIP`.

Dropdown validation can be supplied with `dataValidationRule`, and only with
`columnType: "DROPDOWN"`.

## Inspect Tables

List tables:

```bash
gog sheets table list "$spreadsheet_id"
gog sheets table list "$spreadsheet_id" --json
```

Read one table by ID or name:

```bash
gog sheets table get "$spreadsheet_id" "$table_id"
gog sheets table get "$spreadsheet_id" Tasks --json
```

JSON output includes the table ID, table name, sheet title, A1 range, raw
`GridRange`, and typed columns.

## Append Rows

Append rows by table ID or name:

```bash
gog sheets table append "$spreadsheet_id" "$table_id" \
  --values-json '[["Write docs",2,true]]'
```

Positional values use the same comma-separated row, pipe-separated cell syntax
as `gog sheets append`:

```bash
gog sheets table append "$spreadsheet_id" Tasks 'Write docs|2|true'
gog sheets table append "$spreadsheet_id" Tasks 'One|1|false,Two|2|true'
```

`sheets table append` resolves the table first, then calls the Sheets append API
against the table's bounded A1 range with `INSERT_ROWS`. This lets Sheets place
new rows after the current table data and expand the table, without targeting
the header row directly. Rows wider than the table's column count are rejected
before the mutation is sent.

## Clear Data Rows

Clear table data by table ID or name:

```bash
gog sheets table clear "$spreadsheet_id" "$table_id" --force
```

This clears only the table data body. It never includes the header row in the
clear range. If Sheets reports a footer row, `gog` skips the footer row too and
clears only the rows between header and footer.

Header-only tables fail with a clear message instead of sending an empty or
header-touching mutation. Use `--dry-run --json` to preview the exact data range:

```bash
gog sheets table clear "$spreadsheet_id" Tasks --dry-run --json
```

## Delete A Table

Deleting removes the table object. Use `--force` for non-interactive runs:

```bash
gog sheets table delete "$spreadsheet_id" "$table_id" --force
```

Use `--dry-run` to preview the delete request without mutating the spreadsheet:

```bash
gog sheets table delete "$spreadsheet_id" "$table_id" --dry-run --json
```

## Current Scope

This table command set intentionally covers list, get, create, append, clear
data rows, and delete. Table update and footer editing need separate semantics
because the plain Sheets range APIs can touch table headers or footer rows if
used blindly.

## Command Pages

- [`gog sheets table`](commands/gog-sheets-table.md)
- [`gog sheets table list`](commands/gog-sheets-table-list.md)
- [`gog sheets table get`](commands/gog-sheets-table-get.md)
- [`gog sheets table create`](commands/gog-sheets-table-create.md)
- [`gog sheets table append`](commands/gog-sheets-table-append.md)
- [`gog sheets table clear`](commands/gog-sheets-table-clear.md)
- [`gog sheets table delete`](commands/gog-sheets-table-delete.md)
