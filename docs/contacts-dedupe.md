# Contacts Dedupe

read_when:
- Finding duplicate Google Contacts.
- Reviewing or changing `gog contacts dedupe`.

`gog contacts dedupe` finds likely duplicate personal contacts and prints a
merge plan. Preview is the default; `--apply` performs the reviewed plan.

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

## Apply A Merge

Review the plan first:

```bash
gog contacts dedupe --json
```

Then inspect the exact mutation plan without changing contacts:

```bash
gog contacts dedupe --apply --dry-run --json
```

For automation, copy the contact resource names from the reviewed preview and
scope both dry-run and apply to that exact set:

```bash
gog contacts dedupe \
  --resource people/123 \
  --resource people/456 \
  --apply \
  --dry-run \
  --json
```

Apply interactively:

```bash
gog contacts dedupe --apply
```

Non-interactive automation must explicitly skip confirmation:

```bash
gog contacts dedupe \
  --resource people/123 \
  --resource people/456 \
  --apply \
  --force \
  --json
```

## Output

The command groups contacts that share a matching key. JSON output includes:

- `scanned`: number of contacts examined
- `groups`: likely duplicate groups
- `primary`: the contact gog would keep first in a hypothetical merge plan
- `merged`: merged emails/phones for preview
- `matched_on`: duplicate email/phone/name keys that caused the group
- `members`: all contacts in the group

Applied output also includes:

- `applied`: whether mutations ran
- `groups_merged`: groups completed
- `contacts_deleted`: redundant contacts deleted
- `update_fields`: People API fields unioned into the primary contact
- `delete`: redundant contacts removed after their data was copied

## Safety

`contacts dedupe` remains read-only unless `--apply` is present. Apply mode:

- requires confirmation unless `--force` is present
- honors `--dry-run`
- supports repeatable `--resource` scoping so automation can apply an exact
  reviewed contact set
- reads contact-source data only
- refreshes each contact before planning
- updates the selected primary before deleting anything
- rechecks each redundant contact's etag immediately before deletion
- refuses groups with conflicting singleton fields (`names`, `birthdays`,
  `biographies`, or `genders`)
- refuses secondary contacts with photos or other fields the People API cannot
  preserve through `updateContact`

The People API does not expose Google Contacts' native merge operation. gog
therefore unions all API-updatable fields into the selected primary, then
deletes redundant contacts sequentially. If a request fails, the command stops
and reports completed groups/deletions; copied data remains on the primary and
undeleted contacts remain intact.

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
