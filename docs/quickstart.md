---
title: Quickstart
description: "Five minutes from a clean machine to a working gog setup with one Google account."
---

# Quickstart

Five minutes from a clean machine to authenticated Gmail, Calendar, and Drive
queries. For a deeper look at OAuth clients, service accounts, and named
profiles, read [Auth Clients](auth-clients.md) after this.

## 1. Install

```bash
brew install gogcli
gog --version
```

Other options (Docker, Windows ZIPs, source builds) are documented on
[Install](install.md).

## 2. Get an OAuth client

`gog` talks to Google APIs as you, using your own Cloud project. The one-time
setup is:

1. Open <https://console.cloud.google.com/projectcreate> and create a project.
2. Enable the APIs you intend to use: Gmail, Calendar, Drive, Docs, Sheets,
   Slides, Forms, Apps Script, People (Contacts), Tasks, Classroom — whatever
   you actually need. The [API library](https://console.cloud.google.com/apis/library)
   is the fastest way to enable several at once.
3. Configure the [OAuth consent screen](https://console.cloud.google.com/auth/branding)
   for "External" + your email; that is enough for personal use.
4. Create a **Desktop app** OAuth client at
   <https://console.cloud.google.com/auth/clients> and download the JSON.

Personal `gmail.com` accounts work for normal user APIs (Gmail, Calendar,
Drive, Docs, Sheets, Slides, Forms, Apps Script, Contacts/People, Tasks,
Classroom). Workspace-only APIs (Admin Directory, Cloud Identity Groups, Chat,
Keep with domain-wide delegation) require a managed domain — see
[Auth Clients](auth-clients.md).

> External + Testing OAuth apps issue refresh tokens that expire after seven
> days. Publish the OAuth app for long-lived tokens, or be ready to re-run
> `gog auth add` weekly.

## 3. Store the OAuth client

```bash
gog auth credentials ~/Downloads/client_secret_*.json
```

The file is copied to your per-user config (`$XDG_CONFIG_HOME/gogcli/` or the
OS-equivalent) with mode `0600`.

## 4. Authorize an account

```bash
gog auth add you@gmail.com --services gmail,calendar,drive,docs,sheets,contacts
```

A browser tab opens, you grant the requested scopes, and `gog` stores a
refresh token in your OS keyring (Keychain on macOS, Secret Service on Linux,
Credential Manager on Windows). Headless? Add `--manual` for a paste-the-URL
flow, or `--remote --step 1`/`--step 2` for fully split server runs.

Verify:

```bash
gog auth list --check
gog auth doctor --check
```

## 5. Set a default account

```bash
export GOG_ACCOUNT=you@gmail.com
# or persist a default with gog auth alias
gog auth alias set default you@gmail.com
```

Now you can drop `--account` from every command.

## 6. Run real commands

```bash
# Gmail
gog gmail search 'newer_than:7d' --max 10
gog gmail get <messageId> --sanitize-content --json

# Calendar
gog calendar events --today
gog calendar create --summary "Review" \
  --from "2026-05-06T10:00:00+02:00" \
  --to   "2026-05-06T10:30:00+02:00"

# Drive
gog drive ls --max 20
gog drive tree --parent <folderId> --depth 2
gog drive du   --parent <folderId> --max 20 --json

# Docs / Sheets / Slides
gog docs cat <docId> --tab "Notes"
gog sheets get <spreadsheetId> 'Sheet1!A1:D20' --json
gog slides create-from-markdown "Weekly update" --content-file slides.md

# Profile
gog me
```

`--json` produces a stable JSON envelope on stdout; `--plain` produces TSV.
Human-facing progress, hints, and warnings always go to stderr, so pipes stay
parseable.

## 7. Shell completion (optional)

```bash
gog completion bash    >> ~/.bash_completion
gog completion zsh     >  "${fpath[1]}/_gog"
gog completion fish    >  ~/.config/fish/completions/gog.fish
```

## Where next

- [Auth Clients](auth-clients.md) — named clients, service accounts, ADC,
  Workspace domain-wide delegation, OIDC subject migration.
- [Safety Profiles](safety-profiles.md) — runtime allow/deny lists and baked
  agent-safe binaries.
- [Gmail Workflows](gmail-workflows.md) and [Drive Audits](drive-audits.md) for
  the two surfaces most people start automating.
- [Command Index](commands/) — generated reference for every subcommand.
