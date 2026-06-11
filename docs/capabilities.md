---
title: Runtime capabilities
description: "Discover gog output, auth, safety, command, exit-code, and MCP contracts."
---

# Runtime capabilities

`gog` exposes automation contracts at the top level; there is no separate
agent mode or agent command namespace. Humans, shell scripts, CI jobs, and
agents use the same commands. Machine-readable output, stable exit codes,
non-interactive operation, command guards, and untrusted-content wrapping are
CLI-wide features.

```bash
gog capabilities --json
gog exit-codes --json
gog schema --json
gog mcp --list-tools
```

These commands answer different questions:

| Command | Purpose |
| --- | --- |
| `gog capabilities --json` | What output, auth, and safety behavior is active in this process? |
| `gog schema --json` | Which commands, arguments, and flags does this binary support? |
| `gog exit-codes --json` | Which stable process exit codes can automation branch on? |
| `gog mcp --list-tools` | Which typed MCP tools would this server expose? |

## Capability snapshot

`gog capabilities` reports:

- supported machine output modes and automation flags
- supported authentication methods
- active `--dry-run`, `--no-input`, `--wrap-untrusted`, and
  `--gmail-no-send` state
- baked safety-profile state
- runtime command allowlists and denylists
- commands for schema, exit-code, and MCP tool discovery

The JSON document has a versioned top-level contract:

| Field | Meaning |
| --- | --- |
| `schema_version` | Capability document schema version. Consumers should reject unsupported future versions. |
| `build` | Version and build metadata for the running binary. |
| `disclosure` | Whether credential metadata was inspected or account identity was included. |
| `automation` | Supported machine output modes and canonical safety flags. |
| `auth` | Supported auth methods and, when requested, selected credential metadata. |
| `safety` | Effective runtime flags, baked profile, and command guards. |
| `discovery` | Commands for deeper schema, exit-code, and MCP discovery. |
| `mcp` | MCP-only filtered tool inventory and whether write tools are exposed. |

The default snapshot is local, redacted, and keyring-free. It can indicate that
an account was selected, but does not return the identity, credential type,
scopes, services, or token expiry.

```bash
gog --account you@example.com capabilities --json
```

Credential metadata requires an explicit local CLI request:

```bash
gog --account you@example.com capabilities --include-auth --json
gog --account you@example.com capabilities \
  --include-auth \
  --include-account \
  --json
```

`--include-auth` may open the configured keyring. It returns credential type,
OAuth client name, stored services/scopes, and cached access-token expiry when
available. It never returns access tokens, refresh tokens, client secrets,
service-account key material, or keyring passwords.

`--include-account` is separate because account identity can itself be
sensitive in shared logs.

| Invocation | Opens keyring | Account identity | Credential metadata | Secrets |
| --- | --- | --- | --- | --- |
| `gog capabilities --json` | No | No | No | Never |
| `gog capabilities --include-account --json` | May, when inferring an account | Yes | No | Never |
| `gog capabilities --include-auth --json` | May | No | Yes | Never |
| Both disclosure flags | May | Yes | Yes | Never |
| MCP `gog_capabilities` | No | No | No | Never |

## Automation preflight

Use capabilities as a local preflight before an API command. Apply the same
global safety flags to the preflight and the operation so the reported state
matches the process you intend to run:

```bash
common_flags=(
  --account you@example.com
  --enable-commands-exact capabilities,gmail.search
  --gmail-no-send
  --no-input
  --wrap-untrusted
)

gog "${common_flags[@]}" capabilities --json |
  jq -e '
    .schema_version == 1 and
    .safety.no_input and
    .safety.wrap_untrusted and
    .safety.gmail_no_send and
    (.safety.command_rules.enabled_exact | index("gmail.search"))
  '

gog "${common_flags[@]}" gmail search 'newer_than:7d' --json
```

`capabilities` describes the current invocation. It does not attest to a later
process, test Google API access, refresh OAuth tokens, or prove that a command
will succeed. Use `gog auth list --check --json --no-input` for credential
validation and the relevant read or `--dry-run` command for operational proof.

## Exit codes

`gog exit-codes --json` returns the stable process status map:

| Code | Name | Meaning |
| ---: | --- | --- |
| 0 | `ok` | Success |
| 1 | `error` | Generic or unclassified failure |
| 2 | `usage` | Invalid command syntax, arguments, or flags |
| 3 | `empty_results` | Successful query with no results where empty-result signaling applies |
| 4 | `auth_required` | Missing, expired, revoked, or otherwise unusable authentication |
| 5 | `not_found` | Requested resource does not exist |
| 6 | `permission_denied` | Authenticated caller lacks permission |
| 7 | `rate_limited` | API quota or rate limit reached |
| 8 | `retryable` | Transient server, network timeout, or circuit-breaker failure |
| 10 | `config` | Required local configuration or credentials are missing |
| 11 | `orphaned` | Requested Docs comment is no longer attached to content |
| 130 | `cancelled` | Interrupted with Ctrl-C or context cancellation |

Exit codes classify failures; structured stdout remains command-specific.
Diagnostics stay on stderr. Automation should branch on the documented codes
instead of parsing human error text:

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

Commands that do not classify a failure more specifically return `error` (1).
New classifications may be added, so consumers should retain a generic
non-zero fallback.

## MCP discovery

The read-only `gog_capabilities` MCP tool reports the active safety state and
the filtered tool set registered by that server process. It accepts no input
fields and never performs account or credential disclosure.

If `gog mcp` is started with `--allow-tool`, include `gog_capabilities` or the
`gog` service selector to expose it:

```bash
gog mcp --allow-tool gog_capabilities,docs_get
```

MCP write availability is based on the actual registered tool set after
`--allow-write` and `--allow-tool` filtering.

The MCP result intentionally differs from local CLI disclosure:

- it reports `no_input` and `wrap_untrusted` as enabled because the MCP server
  enforces both
- it includes only tools registered after server-side filtering
- it reports whether any registered tool is write-capable
- it cannot inspect credentials or reveal the configured account

This keeps the model-visible contract controlled by the server operator rather
than by caller-supplied tool arguments.
