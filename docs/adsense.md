# AdSense

read_when:
- Reading AdSense publisher accounts, inventory, payments, or reports.
- Configuring the explicitly opted-in AdSense readonly OAuth grant.

`gog adsense` reads account information, ad clients and units, custom and URL
channels, alerts, payments, policy issues, sites, and reports. It does not
create or modify AdSense resources.

## Authorization and account selection

Enable the AdSense Management API in your OAuth client project and explicitly
authorize its readonly scope when setting up access:

```bash
gog auth add you@example.com --services adsense
```

AdSense is not included in the default `user`, `all-user`, or `all` service
presets. The grant is `https://www.googleapis.com/auth/adsense.readonly`;
the commands do not request the broader management scope. Existing tokens are
not expanded by running a command. The Google identity must already have
access to the publisher account.

The global `--account` selects your **Google login**. Positional account
arguments select an **AdSense publisher**, not another Google login:

```bash
gog --readonly --account you@example.com adsense accounts list --json
gog --readonly --account you@example.com adsense accounts get pub-1234567890123456 --json
```

Publisher arguments accept either `pub-...` or `accounts/pub-...`. Other
resource commands use the full resource name returned by their list command.
List commands that support paging accept `--max`, `--page`, and `--all`.

## Reports

Use a named range, or supply both inclusive custom dates:

```bash
gog --readonly adsense reports query pub-1234567890123456 \
  --date-range YESTERDAY --dimensions DATE --metrics IMPRESSIONS --max 100 --json
gog --readonly adsense reports query pub-1234567890123456 \
  --from 2026-08-01 --to 2026-08-31 --dimensions DATE,COUNTRY_NAME \
  --metrics CLICKS,IMPRESSIONS --timezone GOOGLE_TIME_ZONE --json
```

Custom dates override `--date-range`. Report dimensions and metrics are
case-insensitive; filters and sort expressions are passed through to Google.
JSON includes headers, rows, totals, averages, warnings, and
`total_matched_rows`. Inspect warnings and matched-row counts before treating
a limited result as a complete report.

`--timezone` accepts `ACCOUNT_TIME_ZONE` (the default) or `GOOGLE_TIME_ZONE`
(America/Los_Angeles), case-insensitively. It does **not** accept arbitrary IANA
timezone names. These are Google's [reporting timezone enums](https://developers.google.com/adsense/management/reference/rest/v2/ReportingTimeZone).

Saved reports use their returned full resource name and the same date and
timezone options:

```bash
gog --readonly adsense reports saved list pub-1234567890123456 --json
gog --readonly adsense reports saved query accounts/pub-1234567890123456/reports/report-1 \
  --date-range LAST_7_DAYS --timezone ACCOUNT_TIME_ZONE --json
```

Use `--fail-empty` if an empty result should exit with code 3. Permission errors
retain the standard permission-denied exit code 6. API enablement and OAuth
consent are setup operations; a failed read does not perform either for you.

See the [command reference](commands/gog-adsense.md) for the complete surface.
