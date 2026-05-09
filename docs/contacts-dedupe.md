# Contacts Dedupe Preview

read_when:
- Finding duplicate Google Contacts.
- Reviewing or changing `gog contacts dedupe`.

`gog contacts dedupe` finds likely duplicate personal contacts and prints a
merge plan. It is preview-only: it does not merge, update, or delete contacts.

## Command Page

- [`gog contacts dedupe`](commands/gog-contacts-dedupe.md)

## Basic Use

```bash
gog contacts dedupe
gog contacts dedupe --json
gog contacts dedupe --max 500 --json
```

Default matching uses normalized email and phone values:

```bash
gog contacts dedupe --match email,phone
```

Name matching is opt-in because it can produce false positives:

```bash
gog contacts dedupe --match email,phone,name
```

## Output

The command groups contacts that share a matching key. JSON output includes:

- `scanned`: number of contacts examined
- `groups`: likely duplicate groups
- `primary`: the contact gog would keep first in a hypothetical merge plan
- `merged`: merged emails/phones for preview
- `matched_on`: duplicate email/phone/name keys that caused the group
- `members`: all contacts in the group

## Safety

`contacts dedupe` is read-only. There is no apply flag.

Use `--dry-run` in automation anyway when you want a uniform safety habit across
commands:

```bash
gog contacts dedupe --dry-run --json
```

Use `--fail-empty` in scheduled checks when "no duplicates" should be reported
as a distinct exit code:

```bash
gog contacts dedupe --fail-empty
```

## Related Pages

- [Raw API Dumps](raw-api.md)
- [Generated Contacts command pages](commands/gog-contacts.md)
- [`gog contacts export`](commands/gog-contacts-export.md)
- [`gog contacts raw`](commands/gog-contacts-raw.md)
