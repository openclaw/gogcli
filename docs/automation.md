---
title: Automation
description: "Use gog safely from scripts, CI, and agents with stable output, exit codes, and runtime policy discovery."
---

# Automation

`gog` has one command surface for humans and automation. There is no separate
agent mode or agent command namespace.

Root help summarizes the human-facing contract:

```bash
gog --help
gog help drive ls
```

`gog help <command>` and `gog <command> --help` are equivalent. Once a help
flag is present, trailing arguments are ignored so recovery help remains
available after a malformed command attempt.

The machine-readable contract is:

```bash
gog schema --json
```

The schema contains the complete command tree, arguments, flags, stable exit
codes, output formats, and effective safety state for that invocation.

## Machine output

Use `--json` for structured output or `--plain` for stable TSV. Primary data is
written to stdout; prompts, progress, warnings, and diagnostics are written to
stderr.

```bash
gog --json gmail search 'newer_than:7d'
gog --plain calendar events --today
```

`--results-only` and `--select` transform JSON and therefore require
`--json`. Contradictory output flags fail with usage exit code 2 instead of
being silently ignored. Explicit output flags override `GOG_JSON` and
`GOG_PLAIN` environment defaults. `gog schema` always emits JSON and rejects
`--plain`.

`--fields` is accepted as an alias for `--select` output projection on commands
that do not define their own API field-mask `--fields`; commands with a local
field-mask flag keep that command-specific meaning.

Use `--no-input` in CI and unattended processes. Use `--wrap-untrusted` when
Google-hosted free text will be consumed by an LLM or another instruction-aware
system.

Use `--readonly` (or `GOG_READONLY=1`) as a runtime backstop. It permits GET,
HEAD, OPTIONS, and the small allowlist of Google APIs whose query operations use
POST, while rejecting mutating API requests before network dispatch. This guard
is independent of OAuth scopes and command names, propagates into MCP child
processes, and also blocks Zoom meeting mutations. `gog auth add --readonly`
continues to request read-only OAuth scopes where Google provides them.

```bash
gog --readonly --account you@example.com gmail search 'newer_than:7d'
gog --readonly --account you@example.com calendar freebusy you@example.com
```

Interactive browser commands fail fast under `--no-input`. Preview
`gog auth manage` with `--dry-run`; use `gog auth import` for unattended token
installation.

## Schema automation metadata

The top-level `automation` object has three parts:

| Field | Meaning |
| --- | --- |
| `output_formats` | Stable machine-output modes supported by the CLI. |
| `exit_codes` | Named process exit statuses for branching without parsing stderr. |
| `safety` | Effective runtime flags, command guards, and baked safety profile. |

Example:

```bash
gog \
  --enable-commands-exact schema,gmail.search \
  --gmail-no-send \
  --readonly \
  --no-input \
  --wrap-untrusted \
  schema --json |
  jq '.automation'
```

The safety snapshot describes the current invocation. Apply the same global
flags to the operation:

```bash
common_flags=(
  --account you@example.com
  --enable-commands-exact schema,gmail.search
  --gmail-no-send
  --readonly
  --no-input
  --wrap-untrusted
)

gog "${common_flags[@]}" schema --json |
  jq -e '
    .schema_version == 1 and
    .automation.safety.no_input and
    .automation.safety.wrap_untrusted and
    .automation.safety.gmail_no_send and
    .automation.safety.readonly and
    (.automation.safety.command_rules.enabled_exact | index("gmail.search"))
  '

gog "${common_flags[@]}" gmail search 'newer_than:7d' --json
```

Schema output does not validate credentials, refresh OAuth tokens, test Google
API access, or attest to a later process. Use:

```bash
gog auth list --check --json --no-input
gog auth doctor --check --json --no-input
```

## Exit codes

| Code | Name | Meaning |
| ---: | --- | --- |
| 0 | `ok` | Success |
| 1 | `error` | Generic or unclassified failure |
| 2 | `usage` | Invalid command syntax, arguments, or flags |
| 3 | `empty_results` | Successful query with no results where empty-result signaling applies |
| 4 | `auth_required` | Missing, expired, revoked, or unusable authentication |
| 5 | `not_found` | Requested resource does not exist |
| 6 | `permission_denied` | Authenticated caller lacks permission |
| 7 | `rate_limited` | API quota or rate limit reached |
| 8 | `retryable` | Transient server, network timeout, or circuit-breaker failure |
| 10 | `config` | Required local configuration or credentials are missing |
| 11 | `orphaned` | Requested Docs comment is no longer attached to content |
| 130 | `cancelled` | Interrupted with Ctrl-C or context cancellation |

Malformed local payloads, such as invalid token-import JSON or timestamps, use
`usage` (`2`). Commands that cannot run because their required local setup is
absent or incomplete use `config` (`10`).

The same classifications apply to direct HTTP integrations such as Photos
Library, Photos Picker, and Places. For example, an expired or deleted Picker
session returns `not_found` (`5`) instead of a generic error.

Read the map programmatically:

```bash
gog schema --json | jq '.automation.exit_codes'
```

Automation should branch on exit status rather than human error text:

```bash
if output=$(gog --no-input --json drive get "$file_id"); then
  printf '%s\n' "$output"
else
  rc=$?
  case $rc in
    4)  printf '%s\n' "authentication required" >&2 ;;
    5)  printf '%s\n' "file not found" >&2 ;;
    7|8) printf '%s\n' "retry later" >&2 ;;
    *)  exit "$rc" ;;
  esac
fi
```

New classifications may be added. Keep a generic non-zero fallback.

## MCP discovery

MCP uses its standard `tools/list` request for client-side tool discovery. To
inspect the filtered server surface from a shell before starting it:

```bash
gog mcp --list-tools
gog mcp --allow-tool gmail_search,docs_get --list-tools
```

Write tools remain hidden unless `--allow-write` is set and the tool also
matches `--allow-tool`.
