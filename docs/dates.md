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

## Duration forms

Tracking `--since` also accepts `time.ParseDuration` values such as:

- `24h`
- `15m`

## Agent guidance

- Generate RFC3339 for all datetime fields by default.
- Use date-only for fields explicitly documented as dates.
- For local scheduling, pass an explicit offset (or timezone-aware RFC3339) to avoid ambiguity.
