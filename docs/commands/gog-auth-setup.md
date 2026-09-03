# `gog auth setup`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Guide Google Cloud, OAuth client, and account setup

## Usage

```bash
gog auth setup [<email>] [flags]
```

## Parent

- [gog auth](gog-auth.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--create-project` | `bool` |  | Create --gcloud-project with gcloud (requires confirmation) |
| `--credentials` | `string` |  | Downloaded Desktop OAuth client JSON to store |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-apis` | `bool` |  | Enable selected Google APIs with gcloud |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--force-consent` | `bool` |  | Force OAuth consent when --login runs |
| `--gcloud-project`<br>`--project-id` | `string` |  | Google Cloud project ID (default: active gcloud project) |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--login` | `bool` |  | Run browser OAuth after project/client setup |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--open-console` | `bool` |  | Open the OAuth client page for the selected project |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--project-name` | `string` | gog CLI | Display name when creating a project |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--services` | `string` | gmail,calendar,drive,docs,sheets,contacts | Services to configure: comma-separated gmail,calendar,chat,classroom,drive,driveactivity,drivelabels,docs,slides,contacts,tasks,sheets,people,forms,sites,meet,appscript,analytics,searchconsole,ads,youtube,photos |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog auth](gog-auth.md)
- [Command index](README.md)
