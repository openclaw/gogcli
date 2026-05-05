---
name: gog
description: Use when working with the gog CLI for Google Workspace automation, especially when an agent needs JSON output, auth preflight, command guards, scoped accounts, or safe Google API reads/writes across Gmail, Calendar, Drive, Docs, Sheets, Slides, Forms, Apps Script, Contacts, Tasks, People, Groups, Keep, or Admin.
---

# gog

Use `gog` when built-in Google connectors are missing a feature, when shell
automation needs stable JSON, or when you need to inspect local Google auth
state before acting.

## Fast Path

```bash
gog --version
gog auth list --check --json --no-input
gog auth doctor --check --json --no-input
gog schema --json
```

Pick the account explicitly for API work:

```bash
gog --account user@example.com gmail search 'newer_than:7d' --json
```

Prefer `--json` or `--plain` for agent parsing. Human hints and progress should
stay on stderr; stdout is for data.

## Safety Rules

- Do not print access tokens, refresh tokens, OAuth client secrets, or keyring
  passwords.
- Do not store `GOG_KEYRING_PASSWORD` in a shell profile or plaintext project
  file. If the file keyring cannot unlock non-interactively, stop and ask for a
  safer setup.
- Use `--no-input` in automation so auth/keyring prompts fail clearly.
- Use `--dry-run` first where commands support it.
- Destructive commands require `--force`; do not add it unless the user asked
  for that exact mutation.
- Use `--gmail-no-send` or `GOG_GMAIL_NO_SEND=1` unless sending mail is the
  requested task.
- For shared agent environments, prefer a baked readonly or agent-safe binary
  from `docs/safety-profiles.md`.

Runtime command guards:

```bash
gog --enable-commands gmail.search,gmail.get --gmail-no-send \
  --account user@example.com gmail search 'from:example@example.com' --json

gog --enable-commands drive.ls,docs.cat --disable-commands drive.delete \
  --account user@example.com drive ls --max 10 --json
```

## Auth

OAuth setup is partly interactive. An agent can inspect and diagnose it, but a
human normally completes browser consent:

```bash
gog auth credentials list
gog auth add user@example.com --services gmail,calendar,drive --readonly
gog auth add user@example.com --services docs,sheets,slides
gog auth remove user@example.com
```

Use narrow services and `--readonly` when the task only reads. Service accounts
are Workspace-only and mainly fit Admin, Groups, Keep, and domain-wide
delegation flows; they do not solve consumer `@gmail.com` OAuth.

## Common Reads

```bash
gog --account user@example.com gmail search 'newer_than:3d' --max 10 --json
gog --account user@example.com gmail get <messageId> --sanitize-content --json
gog --account user@example.com gmail thread get <threadId> --sanitize-content --json

gog --account user@example.com calendar events --today --json
gog --account user@example.com drive ls --max 20 --json
gog --account user@example.com docs cat <documentId> --json
gog --account user@example.com sheets get <spreadsheetId> Sheet1!A1:D20 --json
gog --account user@example.com contacts list --max 20 --json
```

For Gmail body inspection, prefer `--sanitize-content` unless the user
explicitly needs raw payloads.

## Writes

Before writes, identify the account, object id, and exact mutation. Prefer
commands that support `--dry-run`, and clean up disposable live-test objects.

```bash
gog --account user@example.com docs write <documentId> --append --text '...'
gog --account user@example.com sheets update <spreadsheetId> Sheet1!A1 --values-json '[["hello"]]'
gog --account user@example.com drive upload ./file.txt --parent <folderId> --json
```

When testing creation commands, name artifacts with a clear temporary prefix and
delete or trash them after verification.

## Discovery

Use generated command docs and schema instead of guessing flags:

```bash
gog <service> --help
gog <service> <command> --help
gog schema <service> <command> --json
```

Docs:

- `docs/README.md`
- `docs/commands/README.md`
- `docs/safety-profiles.md`
- `README.md#security`

Repo paths:

- CLI entrypoint: `cmd/gog/`
- Command implementations: `internal/cmd/`
- OAuth/keyring: `internal/auth/`, `internal/secrets/`
- Generated command docs: `docs/commands/`
