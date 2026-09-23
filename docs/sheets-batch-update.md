# Sheets Batch Updates

Use `gog sheets batch-update` when you need to update multiple ranges in the
same spreadsheet without making one API call per range. The command sends a
single Google Sheets `spreadsheets.values.batchUpdate` request.

Prepare a JSON array of value ranges:

```json
[
  {
    "range": "Sheet1!A1:B1",
    "values": [["Name", "Status"]]
  },
  {
    "range": "Sheet1!A2:B3",
    "values": [
      ["Ada", "Ready"],
      ["Grace", "Blocked"]
    ]
  }
]
```

Then pass it inline or from a file:

```bash
gog sheets batch-update "$spreadsheet_id" --data-json @updates.json --json
```

By default, values are interpreted as if they were entered in the Google Sheets
UI (`USER_ENTERED`). Use `--input RAW` to store values without parsing:

```bash
gog sheets batch-update "$spreadsheet_id" \
  --input RAW \
  --data-json '[{"range":"Sheet1!A1:B1","values":[["001","plain text"]]}]'
```

Add `--include-values-in-response` when callers need the post-update cell values
back from Google:

```bash
gog sheets batch-update "$spreadsheet_id" \
  --include-values-in-response \
  --response-render UNFORMATTED_VALUE \
  --data-json @updates.json \
  --json
```

Related command reference:

- [`gog sheets batch-update`](commands/gog-sheets-batch-update.md)
- [`gog sheets update`](commands/gog-sheets-update.md)

## Structural batch requests

Use `gog sheets batch-request` to submit formatting, tab, dimension, and other
structural operations together through `spreadsheets.batchUpdate`. The existing
`batch-update` command and `batch` alias continue to update values only.

Supply a nonempty JSON array of Google Sheets request objects, inline, from a
file, or from stdin with `@-`:

```json
[
  {
    "updateSheetProperties": {
      "properties": {"sheetId": 0, "gridProperties": {"frozenRowCount": 1}},
      "fields": "gridProperties.frozenRowCount"
    }
  },
  {
    "repeatCell": {
      "range": {"sheetId": 0, "startRowIndex": 0, "endRowIndex": 1},
      "cell": {"userEnteredFormat": {"textFormat": {"bold": false}}},
      "fields": "userEnteredFormat.textFormat.bold"
    }
  }
]
```

```bash
gog sheets batch-request "$spreadsheet_id" --requests-json @requests.json --dry-run --json
gog sheets batch-request "$spreadsheet_id" --requests-json @requests.json --force --json
```

The dry run prints the complete request array without authentication or a write.
Execution requires confirmation, or `--force` in automation, because the raw
endpoint can delete data. Explicit zero, false, empty-string, and null values
are preserved. Google validates operations and applies the ordered batch
atomically: if any request is invalid, none of its changes are applied.
`--json` returns the complete Google response, including ordered `replies`.

With command restrictions, explicitly allow `sheets.batch-request`; a parent
`sheets` grant or a wildcard combined with deny rules is insufficient. Baked
profiles use the same explicit grant and cannot be widened by runtime flags.
The bundled restricted profiles do not grant this command. An explicit grant
authorizes the entire structural endpoint, including deletion: denying a sibling
command such as `sheets.delete-tab` does not filter the raw request array.
`--readonly` always blocks execution, and `--force` never bypasses policy.

The command makes one submission, with no automatic retries, redirects, or
splitting. A network or server error can follow a completed write; inspect the
spreadsheet before retrying. This command does not persist a queued batch or
provide revision locking.

## Single-range formula verification

For a single updated range, `--values-json` accepts inline JSON, `@file`, or
`@-` for stdin. File or stdin input avoids shell history expansion and quoting
problems for formulas containing `!`:

```bash
printf '%s\n' '[["=Sheet2!C9"]]' >formula.json
gog sheets update "$spreadsheet_id" 'Sheet1!B13' \
  --values-json @formula.json \
  --fail-on-formula-error \
  --json
```

With `--fail-on-formula-error`, the command reads the exact updated range back
as structured grid data. It exits nonzero when Sheets reports an effective
cell error and returns `formulaErrors` entries with the cell, error type, and
message. Literal strings stored with `--input RAW`, such as `#REF!`, remain
valid because verification uses the API's typed error value instead of matching
displayed text.
