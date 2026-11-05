# Date and Time Input Formats

Use one parsing contract across commands.

## Canonical choices

- Prefer `RFC3339` in automation: `2026-02-13T15:04:05Z`
- Use `YYYY-MM-DD` for date-only fields (birthdays, date-only due values)
- Keep timezone explicit when time matters

## Accepted input formats

- Date-only: `YYYY-MM-DD`
- Datetime: `RFC3339` / `RFC3339Nano`
- ISO offset without colon: `YYYY-MM-DDTHH:MM:SS-0800`
- Local datetime (no timezone):
  - `YYYY-MM-DDTHH:MM[:SS]`
  - `YYYY-MM-DD HH:MM[:SS]`

## Relative forms

Calendar range flags (`--from` / `--to`) also accept:

- `now`, `today`, `tomorrow`, `yesterday`
- Weekday names: `monday`, `next friday`

## Calendar windows

Calendar range commands reject equal or reversed bounds as usage errors before
requesting events. Bounds must span at least one second at the API's precision.
Date-only `--to` values include that whole day.

`calendar search` defaults to 30 days before now through 90 days after now. A
historical `--from` with no `--to` keeps the end at now + 90 days. If an explicit
start would meet or exceed that default end, the search instead ends 90 days
after the start. Pass both bounds for an exact window.

## Duration forms

Tracking `--since` also accepts `time.ParseDuration` values such as:

- `24h`
- `15m`

## Agent guidance

- Generate RFC3339 for all datetime fields by default.
- Use date-only for fields explicitly documented as dates.
- For local scheduling, pass an explicit offset (or timezone-aware RFC3339) to avoid ambiguity.
