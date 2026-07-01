# `gog forms add-question`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Add a question to a form

## Usage

```bash
gog forms (form) add-question (add-q,aq) --title=STRING <formId> [flags]
```

## Parent

- [gog forms](gog-forms.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--correct` | `[]string` |  | Correct answer value for quiz grading (repeat for multiple accepted/checkbox answers) |
| `--description` | `string` |  | Question description/help text |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--duration` | `bool` |  | Ask for duration instead of time (for time type) |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `--include-time` | `bool` |  | Include time picker (for date type) |
| `--include-year` | `bool` |  | Include year field (for date type) |
| `--index` | `int` | -1 | Position to insert (0-based, default append) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-o`<br>`--option` | `[]string` |  | Choice options (for radio/checkbox/dropdown, repeat for each) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--points` | `int` |  | Positive quiz points for the question when --correct is set |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--required` | `bool` |  | Whether an answer is required |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--scale-high` | `int` | 5 | Scale maximum value: 2 through 10 |
| `--scale-high-label` | `string` |  | Label for high end of scale |
| `--scale-low` | `int` | 1 | Scale minimum value: 0 or 1 |
| `--scale-low-label` | `string` |  | Label for low end of scale |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--title` | `string` |  | Question title/text |
| `--type` | `string` | text | Question type: text\|paragraph\|radio\|checkbox\|dropdown\|scale\|date\|time |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog forms](gog-forms.md)
- [Command index](README.md)
