# Import CSV or TSV into an existing sheet

```sh
gog sheets import SHEET_ID 'Data!A1:D1000' --file data.csv --dry-run --json
gog sheets import SHEET_ID 'Data!A1:D1000' --file data.tsv --format tsv --json
gog sheets import SHEET_ID 'Data!A1:F1000' --file data.csv --match-headers --json
```

Use a bounded destination rectangle including the sheet name. Input may be a file or `--file -` for stdin. CSV and TSV support quoted fields, embedded newlines, CRLF, and a UTF-8 BOM. Input is limited to 32 MiB. Empty input, malformed quoting, inconsistent row widths, or values exceeding the rectangle fail before writes.

All imported cells use RAW strings: leading zeros, date-like text, and strings beginning with `=` stay literal. Without header matching, the entire file, including any header, is written positionally. Existing cells outside the imported rectangle are not cleared, including old rows below shorter input.

With `--match-headers`, the file's first row and destination's first row supply exact, case-sensitive header names. Only matching input columns are updated, beneath the existing header; unmentioned destination columns are preserved. Missing or duplicate headers fail. This mode reads destination headers even during dry runs; positional dry runs remain offline.

The command submits one values batch using the existing batch-update implementation. For a new spreadsheet created by CSV conversion, the existing `drive upload --convert-to sheet` remains available.
