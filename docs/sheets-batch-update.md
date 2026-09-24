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

Without `--batch`, the command makes one submission, with no automatic retries,
redirects, or splitting. A network or server error can follow a completed write;
inspect the spreadsheet before retrying. Sheets does not provide revision locking.

## Persisted structural batches

Queue structural edits across CLI invocations, inspect them, and submit one
atomic `spreadsheets.batchUpdate`:

```bash
BATCH_ID="$(gog --account you@example.com batch begin --spreadsheet "$spreadsheet_id")"
gog --account you@example.com sheets format "$spreadsheet_id" 'Report!A1:H1' \
  --format-json '{"textFormat":{"bold":true}}' --batch "$BATCH_ID"
gog --account you@example.com sheets freeze "$spreadsheet_id" --sheet Report \
  --rows 1 --batch "$BATCH_ID"
gog --account you@example.com sheets resize-columns "$spreadsheet_id" 'Report!A:H' \
  --width 120 --batch "$BATCH_ID"
gog batch show "$BATCH_ID" --json
gog batch end "$BATCH_ID" --dry-run --json
gog batch end "$BATCH_ID" --force --json
```

Use exactly one target: `--spreadsheet`, `--doc`, `--presentation`, or `--form`.
`--service sheets` is optional and must match the target. `begin` binds the
spreadsheet, selected account, and OAuth client without contacting Google.
Submissions use that stored identity. Direct-token and ADC modes still use the
active credential; account labels do not verify its principal.

`--batch` is supported by `format`, `number-format`, `conditional-format add`,
`merge`, `unmerge`, `freeze`, `resize-columns`, `resize-rows`, `filter set`,
`update-note`, `links set`, `chart create/update/delete`,
`named-ranges add/update/delete`, `add-tab`, `rename-tab`, `delete-tab`,
`find-replace`, `banding set/clear`, `table create/delete`, and `insert`.
It is also supported by `batch-request` for a complete raw request array.
Omitting the flag keeps immediate behavior. An explicitly empty flag is rejected
before input or authentication; malformed UUIDs are rejected even in dry runs.

Queueing returns `batch_id`, the number `queued`, and total `requests`.
It does not claim that edits were applied or return IDs/counts that only Google
can assign. Destructive execution is confirmed once at `batch end`, or authorized
with `--force`; semantic guards such as `table delete --discard-data` still apply
while queueing. `--readonly` permits local preparation and blocks submission.

### One immutable base metadata snapshot

The first typed command needing name/range resolution captures a narrow metadata
snapshot under the batch's account, client, and spreadsheet binding. Later CLI
invocations reuse it, so 120 formatting commands require one metadata read rather
than 120. The snapshot includes existing sheet titles/IDs/grid bounds, named
ranges, filter ranges, and chart, banding, and table identities. It contains no
cell values, formulas, chart specifications, or table column definitions.
Raw-only batches and commands needing no lookup do not capture metadata.

Typed names and ranges always resolve against this original base. The snapshot
is never refreshed, and queued edits are not replayed locally. A queued rename
does not change subsequent name lookup; a queued new tab cannot be resolved by
title. Use raw requests with explicit IDs for dependencies on queued changes:

```bash
gog sheets batch-request "$spreadsheet_id" --batch "$BATCH_ID" --requests-json \
  '[{"addSheet":{"properties":{"sheetId":123456,"title":"New report"}}},
    {"repeatCell":{"range":{"sheetId":123456},"cell":{"note":"Queued together"},"fields":"note"}}]'
```

Raw request JSON preserves zero, false, empty, null, large numeric, and unknown
fields through queueing and submission. The caller is responsible for explicit
IDs and ordered dependencies. Choose a new batch to capture newer metadata.

`conditional-format clear`, `delete-dimension`, `reorder-tab`, `duplicate-tab`,
`validation set/clear`, and `copy-paste` do not accept `--batch`: their convenience
logic derives rule indexes, tab positions, or table repairs from live state that
earlier queued changes could invalidate. Their underlying Sheets requests remain
available through `batch-request --batch`. Values operations (including table
append/clear), reads, file creation/copy/export, and Connected Sheets operations
remain outside this structural queue.

### Submission guarantees and recovery

Sheets validates the ordered request array and applies it atomically by default.
It has no Docs/Slides revision precondition: the captured base is a lookup aid,
not a concurrency lock. Collaborator changes can affect the result. No synthetic
revision, Drive version, or ETag is used to suggest stronger protection.

Submitting a Sheets batch requires the explicit `sheets.batch-request` capability
under runtime or baked command restrictions, in addition to permission for
`batch end`. An existing `batch` or parent `sheets` grant does not grant the raw
endpoint. This permission covers all structural requests, including deletion;
sibling deny rules do not filter the stored JSON. `--force` cannot bypass it.

`gog` limits one persisted atomic submission to 500 requests. `--auto-split`
explicitly opts into ordered chunks of at most 500. `--continue-on-error` opts into
individual recovery only after Google rejects the atomic batch with HTTP 400.
These modes cannot be combined and are non-atomic. Use individual recovery only
for independent requests: skipping a failed earlier insert/delete can change
the meaning of later positional edits. Prefer repairing the atomic batch when
requests depend on each other.

Successful chunks/requests are removed from local state; confirmed individual
HTTP 400 failures remain, and retained failures produce a nonzero exit code.
An individual transport, server, or response-decoding failure stops recovery
immediately, retaining earlier failures, the uncertain request, and untouched
remaining requests. Inspect the spreadsheet before manually retrying: a lost
response can follow a successful write. Submissions never retry automatically
or follow redirects.

Dry runs validate local inputs and print the planned batch target without
authentication, metadata reads, or state writes. Typed previews use the supplied
names/ranges; `batch show` and `batch end --dry-run` expose the exact queued wire
payload. `batch abort` and `batch prune` retain their existing local lifecycle.

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
