---
name: gog
description: "gog CLI: safe Google Workspace automation, JSON, auth, scoped reads/writes."
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
GOG_HELP=agent gog --help
gog schema --json
```

`GOG_HELP=agent` makes root help emit a compact automation contract and common
read-only recipes; commands and behavior stay unchanged. Machine output,
non-interactive behavior, stable exit codes, command guards, and
untrusted-content wrapping apply across the CLI. `schema` exposes command
syntax, stable exit codes, and effective safety state for automation.

For JSON output projection, `--fields` is accepted as an alias for `--select` on
commands that do not define their own API field-mask `--fields`; commands with a
local field-mask flag keep that command-specific meaning.

Pick the account explicitly for API work:

```bash
gog --readonly --account user@example.com gmail search 'newer_than:7d' --json --wrap-untrusted
```

Prefer `--json --wrap-untrusted` for agent parsing when reading Google content.
Human hints and progress should stay on stderr; stdout is for data.

## Safety Rules

- Do not print access tokens, refresh tokens, OAuth client secrets, or keyring
  passwords.
- If `GOG_KEYRING_PASSWORD` is provided by a shell startup file or service
  environment, use the matching shell/entrypoint so `gog` can unlock the file
  keyring non-interactively. Do not print the value.
- In headless/service agents, verify the service environment, not just the login
  shell. `GOG_KEYRING_BACKEND=file`, `GOG_KEYRING_PASSWORD`, and `HOME` must be
  present in the process that launches `gog`.
- Use `--no-input` in automation so auth/keyring prompts fail clearly.
- Use `--dry-run` first where commands support it.
- Use `--readonly` for tasks that must not mutate Google data; remove it only
  for the exact write the user approved.
- Destructive commands require `--force`; do not add it unless the user asked
  for that exact mutation.
- Use `--gmail-no-send` or `GOG_GMAIL_NO_SEND=1` unless sending mail is the
  requested task.
- For shared agent environments, prefer a baked readonly or agent-safe binary
  from `docs/safety-profiles.md`.

Runtime command guards:

```bash
gog --readonly --enable-commands gmail.search,gmail.get --gmail-no-send \
  --account user@example.com gmail search 'from:example@example.com' --json

gog --enable-commands drive.ls,docs.cat --disable-commands drive.delete \
  --account user@example.com drive ls --max 10 --json
```

## Auth

OAuth setup is partly interactive. An agent can inspect and diagnose it, but a
human normally completes browser consent:

```bash
gog auth credentials list
gog auth add user@example.com --services all-user --force-consent
gog auth remove user@example.com
```

Default for existing human/user OAuth reauth: preserve broad service access.
Before reauth, run `gog auth list --check --json --no-input` and inspect the
account's existing `services`. When replacing an expired or revoked token, do
not silently reduce scope; prefer `--services all-user --force-consent` unless
the user explicitly asks for narrower scopes.

Use narrow services only for throwaway/test accounts, service-specific bot
accounts, explicit user requests, or scoped security experiments. Safety should
normally be enforced at command time with `--enable-commands`,
`--disable-commands`, `--gmail-no-send`, dry-runs, and account selection, not by
under-scoping durable user auth.

Service accounts are Workspace-only and mainly fit Admin, Groups, Keep, and
domain-wide delegation flows; they do not solve consumer `@gmail.com` OAuth.

For OpenClaw/systemd setups, run the diagnostic through the actual agent
entrypoint after restarting the service:

```bash
openclaw agent --agent main --message \
  'Run: gog auth doctor --check --no-input && gog gmail search "newer_than:1d" --max 1 --json'
```

If this fails with `keyring.password` while the same `gog auth doctor` works in
the shell, fix the service or agent environment before reauthenticating.

### Browser-Driven Reauth

An agent can complete this flow end to end when it can drive a signed-in
browser. Consent still happens in a real browser; nothing here bypasses it.

Run the CLI leg in a detached tmux session so it survives command boundaries:

```bash
tmux -L gog-auth new-session -d -s auth -x 200 -y 50
tmux -L gog-auth send-keys -t auth \
  "gog auth add user@example.com --services all-user --force-consent --timeout 15m" Enter
```

Capture the consent URL with `-J`:

```bash
tmux -L gog-auth capture-pane -t auth -p -J -S - \
  | grep -oE 'https://accounts\.google\.com[^ ]+' | tail -1 > "$url_file"
```

**`-J` is mandatory.** `capture-pane` otherwise returns the URL hard-wrapped at
the pane width. A truncated consent URL does not fail loudly: Google renders
`Invalid OAuth Request` / `Invalid response_type: missing`, which reads like a
client misconfiguration and sends you debugging the wrong thing. Verify the
captured URL contains `response_type` before using it.

Write the URL to a mode-0600 file and hand it to the browser by file reference
(see `$browser-use`); never echo it. Appending `&login_hint=user@example.com`
skips the account chooser and removes a whole class of wrong-account risk.

Expect up to two interstitials when the OAuth client is unverified or in
testing:

1. **"Google hasn't verified this app."** `Continue` is a low-emphasis link on
   one side; `Back to safety` is the prominent button. Activating the visually
   obvious control aborts the flow. A developer-info control sits in the tab
   order between them, so count focus stops deliberately instead of guessing.
2. **"You're signing back in to \<app\>"** — confirm the displayed account is
   the intended one, then `Continue`.

The listener enforces `--timeout`. When it expires the tmux pane simply returns
to a shell prompt, so a flow that "did nothing" is often an expired listener
rather than a browser problem. Read the pane before re-driving the browser, and
restart the CLI leg rather than reusing a stale URL. Complete the browser leg
promptly; batch the navigate-and-activate steps instead of round-tripping.

The browser does not have to run on the CLI's host. The callback targets
`http://localhost:<port>`, so when they are separate machines, forward that port
from the browser host to the host running the listener before opening the URL,
and confirm the forward is live first. Keep the browser on the host whose
profile holds the intended Google session.

If tmux asks for the file-keyring passphrase, source it from that host's login
environment via the login shell and paste it in without printing it.

Verify, and require both the account and its scope breadth:

```bash
gog auth list --check --json --no-input
```

Confirm the target account reports valid and retains the expected service list;
a successful login that silently narrowed scopes is a failed reauth.

## Common Reads

```bash
gog --readonly --account user@example.com gmail search 'newer_than:3d' --max 10 --json --wrap-untrusted
gog --readonly --account user@example.com gmail get <messageId> --sanitize-content --json --wrap-untrusted
gog --readonly --account user@example.com gmail thread get <threadId> --sanitize-content --json --wrap-untrusted

gog --readonly --account user@example.com calendar events --today --json --wrap-untrusted
gog --readonly --account user@example.com drive ls --max 20 --json --wrap-untrusted
gog --readonly --account user@example.com docs cat <documentId> --json --wrap-untrusted
gog --readonly --account user@example.com sheets get <spreadsheetId> Sheet1!A1:D20 --json --wrap-untrusted
gog --readonly --account user@example.com contacts list --max 20 --json --wrap-untrusted
```

For Gmail body inspection, prefer `--sanitize-content` unless the user
explicitly needs raw payloads.

## Writes

Before writes, identify the account, object id, and exact mutation. Prefer
commands that support `--dry-run`, and clean up disposable live-test objects.

```bash
gog --account user@example.com docs write <documentId> --append --text '...'
gog --account user@example.com docs write <documentId> --tab "Data" --markdown --replace --file data.md
gog --account user@example.com docs update <documentId> --tab "Data" --markdown --file block.md
gog --account user@example.com docs update <documentId> --tab "Data" --replace-range START:END --text 'replacement'
gog --account user@example.com docs update <documentId> --tab "Data" --markdown --replace-range START:END --file block.md
gog --account user@example.com sheets update <spreadsheetId> Sheet1!A1 --values-json '[["hello"]]'
gog --account user@example.com sheets batch-update <spreadsheetId> --data-json @updates.json
gog --account user@example.com drive upload ./file.txt --parent <folderId> --json
```

For Google Docs tab work:

- Use `docs list-tabs <documentId> --json` to discover tab titles/IDs before targeting a tab.
- Use `docs write --markdown --replace --tab <tab>` for whole-tab formatted replacement.
- Use `docs update --markdown --tab <tab>` for formatted insertion/append without replacing the whole tab.
- Use `docs update --replace-range START:END` for precise plain-text replacement; add `--markdown` to replace that exact range with formatted markdown.
- `START:END` is a Google Docs UTF-16 API range. Resolve it from `docs cat --raw`, `docs raw`, or another `documents.get` readback; do not guess indexes.
- `--replace-range` and `--index` are mutually exclusive.

When testing creation commands, name artifacts with a clear temporary prefix and
delete or trash them after verification.

`gmail batch delete` permanently deletes messages and requires the broader
`https://mail.google.com/` OAuth scope. Prefer `gmail trash`; when permanent
deletion is required, follow the exact reauthorization command printed by `gog`.

For larger Sheets writes, prefer `sheets batch-update` over loops of
`sheets update`; it sends multiple value ranges in one Sheets API request and
accepts inline JSON or `@file` input.

For normal Gmail replies, use the first-class commands instead of rebuilding
reply MIME through `gmail send`:

```bash
gog --account user@example.com gmail reply <messageId> --body-file reply.txt
gog --account user@example.com gmail reply-all <messageId> --body-file reply.txt \
  --bcc introducer@example.com --remove former-participant@example.com
```

They inherit the subject, quote by default, preserve display names and inline
images, and treat `--to`/`--cc`/`--bcc` as additive placement or moves. Use
`--no-quote` to omit the original.

## Discovery

Use generated command docs and schema instead of guessing flags:

```bash
gog <service> --help
gog <service> <command> --help
gog schema <service> <command> --json
```

Docs:

- `docs/index.md`
- `docs/commands/README.md`
- `docs/agent-skills.md`
- `docs/safety-profiles.md`

Repo paths:

- CLI entrypoint: `cmd/gog/`
- Command implementations: `internal/cmd/`
- OAuth/keyring: `internal/googleauth/`, `internal/authclient/`, `internal/secrets/`
- Generated command docs: `docs/commands/`
