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
