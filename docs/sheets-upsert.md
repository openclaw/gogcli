# Keyed Sheets upserts

Use `sheets upsert` when automation and people maintain different columns of the same table.

```sh
gog sheets upsert SHEET_ID 'Data!A1:F5000' --key id --columns status,score \
  --rows-json @rows.json --dry-run --json
gog sheets upsert SHEET_ID 'Data!A1:F5000' --key id --columns status,score \
  --rows-json @rows.json --json
```

The input is an array of objects, for example `[{"id":"001","status":"done","score":3}]`. The first row of the explicit, bounded destination is its header. Header matching is exact and case-sensitive. Keys must be unique, nonempty literal strings on both sides; import identifiers as RAW text to preserve leading zeros.

Only named update columns are written on matched rows. The key is immutable on existing rows. Unselected columns, formatting, formulas, and rows absent from the input are left alone. A missing field or JSON null skips a cell; an empty string clears it. New keys are written after the last populated row within the provided bounds, with unselected columns left blank. No rows are deleted, reordered, or structurally inserted.

Duplicate headers or keys, unselected input fields, non-scalar fields, and capacity overflow fail before submitting any write. Dry runs read the table and return exact cell updates without writing. The table is reread before applying; detected drift aborts. Sheets has no revision precondition for this values request, so a concurrent edit after the reread is still possible. Pause concurrent editing when row positions must remain stable. An API error can leave completion unknown: read the table before retrying.
