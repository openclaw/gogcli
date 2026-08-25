---
title: Connected Sheets
description: Inspect BigQuery and Looker data sources, execution status, and anchored Connected Sheets extracts without mutating the spreadsheet.
---

# Connected Sheets

`gog sheets datasource` provides a read-only view of external data sources in a spreadsheet. It can list sources, return the complete source specification and execution status, discover anchored data-source tables (called extracts in the Sheets editor), and read a bounded number of extract rows.

## Authorize BigQuery access explicitly

Google requires `https://www.googleapis.com/auth/bigquery.readonly` whenever a Sheets API response contains BigQuery Connected Sheets data. Ordinary `sheets` authorization intentionally does not request that scope.

Re-authorize the account with its existing service selection, append the scope, and force the consent screen. For a Sheets-only token:

```bash
gog auth add you@example.com \
  --services sheets \
  --extra-scopes https://www.googleapis.com/auth/bigquery.readonly \
  --force-consent
```

If the account token covers more services, keep that existing `--services` selection instead of narrowing it to `sheets`. Domain-wide delegated service accounts must also have both the Sheets read-only and BigQuery read-only scopes approved by the Workspace administrator; the Connected Sheets client requests only those two scopes.

Looker data sources reuse the account's existing Looker link, but the same commands and output shape apply.

## List and describe data sources

```bash
gog --readonly --account you@example.com \
  sheets datasource list <spreadsheetId>

gog --readonly --account you@example.com \
  sheets datasource describe <spreadsheetId> <dataSourceId> --json
```

`list` returns a compact source summary joined with its `DATA_SOURCE` sheet and current `DataExecutionStatus`. It deliberately does not print custom SQL. `describe` returns the complete API `DataSource`, associated sheet properties, execution status, and refresh schedules, so its JSON can include a BigQuery raw query and error messages.

## Discover and read extracts

A data-source table has no standalone ID in the Sheets API. Its definition lives only on the table's top-left anchor cell, so the CLI identifies extracts with an A1 anchor that includes the sheet name.

```bash
gog --readonly --account you@example.com \
  sheets datasource table list <spreadsheetId>

gog --readonly --account you@example.com \
  sheets datasource table describe <spreadsheetId> 'Extracts!B3' --json

gog --readonly --account you@example.com \
  sheets datasource table read <spreadsheetId> 'Extracts!B3' \
  --max-rows 250 --json
```

Table discovery asks `spreadsheets.get` only for anchor definitions and related sheet metadata. `read` then uses the selected table's configured columns and row limit to construct a bounded `spreadsheets.values.get` request. The default is at most 1,000 data rows plus the header; JSON output reports `truncated: true` when the configured extract can contain more rows. Use `--render FORMULA` or `--render UNFORMATTED_VALUE` when formatted display values are not suitable.

An extract that syncs every column keeps its column list on the linked `DATA_SOURCE` sheet rather than on the anchor, and the anchor lookup is range-scoped, so `read` issues one additional `spreadsheets.get` for those column definitions. Add pacing when reading many extracts in a loop; back-to-back reads can reach the Sheets per-minute quota.

These commands do not create, update, refresh, or delete data sources. Connected Sheets refresh remains asynchronous, and `list` or `describe` can be polled until `state` is `SUCCEEDED` or `FAILED`.
