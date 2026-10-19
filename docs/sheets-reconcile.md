# Preview and verify Sheets values

```sh
gog sheets reconcile SHEET_ID --data-json @ranges.json --json
gog sheets reconcile SHEET_ID --data-json @ranges.json --apply --json
```

The default is a live read-only preview. Input is a ValueRange array, such as `[{"range":"Data!A1:C2","values":[["001",null,""]]}]`. Each range must be a bounded A1 rectangle with an explicit sheet name. Only row-major data is supported. JSON null skips a cell, an empty string clears it, and omitted cells remain untouched. Overlapping explicit cells are rejected.

The preview returns each changed cell's address, current effective value, and desired value. Apply rereads the target ranges, refuses detected drift, writes only changed cells with RAW semantics, and reads back every explicitly supplied value. A mismatch produces a nonzero exit status with structured mismatch details. API errors after submission can leave completion unknown; preview again before retrying.

Comparison uses unformatted effective values and numeric date serials, so existing formulas that already evaluate to the desired value are retained. Literal strings beginning with equals are written as text. It does not compare cell formatting, formula source, or unmentioned cells. There is no atomic Sheets compare-and-swap; pause concurrent editing when it matters. `--dry-run` overrides `--apply` and never writes.
