# `gog classroom`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Google Classroom

## Usage

```bash
gog classroom (class) <command> [flags]
```

## Parent

- [gog](gog.md)

## Subcommands

- [gog classroom announcements](gog-classroom-announcements.md) - Announcements
- [gog classroom courses](gog-classroom-courses.md) - Courses
- [gog classroom coursework](gog-classroom-coursework.md) - Coursework
- [gog classroom guardian-invitations](gog-classroom-guardian-invitations.md) - Guardian invitations
- [gog classroom guardians](gog-classroom-guardians.md) - Guardians
- [gog classroom invitations](gog-classroom-invitations.md) - Invitations
- [gog classroom materials](gog-classroom-materials.md) - Coursework materials
- [gog classroom profile](gog-classroom-profile.md) - User profiles
- [gog classroom roster](gog-classroom-roster.md) - Course roster (students + teachers)
- [gog classroom students](gog-classroom-students.md) - Course students
- [gog classroom submissions](gog-classroom-submissions.md) - Student submissions
- [gog classroom teachers](gog-classroom-teachers.md) - Course teachers
- [gog classroom topics](gog-classroom-topics.md) - Topics

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog](gog.md)
- [Command index](README.md)
