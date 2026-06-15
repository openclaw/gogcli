---
title: Overview
permalink: /
description: "gog is a single Go CLI for Gmail, Calendar, Drive, Docs, Sheets, Slides, Forms, Apps Script, Contacts, Tasks, and Workspace admin — built for terminals, scripts, CI, and coding agents."
---

## Try it

After you store an OAuth client and authorize an account ([Quickstart](quickstart.md) walks through the five-minute version), everything is a one-liner.

```bash
# Search this week's mail and read a sanitized message body for an agent.
gog gmail search 'newer_than:7d' --max 10
gog gmail get <messageId> --sanitize-content --json

# Today's calendar.
gog calendar events --today

# Workspace user and org-unit management with Admin SDK / domain-wide delegation.
gog --account admin@example.com admin users create ada@example.com \
  --first-name Ada --last-name Lovelace --change-password
gog --account admin@example.com admin orgunits list --type all

# Audit a Drive folder without changing anything.
gog drive tree --parent <folderId> --depth 2
gog drive du   --parent <folderId> --max 20 --json

# Edit a Doc, append to a Sheet table, push slides from Markdown.
gog docs format <docId> --match Status --bold --font-size 18
gog sheets table append <spreadsheetId> Tasks 'Ship README|done'
gog slides create-from-markdown "Weekly update" --content-file slides.md
```

`--json` produces a stable JSON envelope on stdout, `--plain` produces TSV; human progress, prompts, and warnings always go to stderr so pipes stay parseable.

## What gog does

- **One binary, every API.** Gmail, Calendar, Drive, Docs, Sheets, Slides, Forms, Apps Script, Contacts, People, Tasks, Classroom, Chat, Groups, Keep, and Workspace Admin.
- **Stable output.** `--json` for scripts, `--plain` TSV for `awk`, human output on stderr.
- **Runtime discovery.** `gog schema --json` exposes command shape, stable exit codes, output modes, and effective safety state.
- **Multi-account, multi-client.** Many Google accounts and OAuth client projects in one config; OAuth, direct access tokens, ADC, and Workspace service accounts all supported.
- **One automation contract.** Humans, scripts, CI, and agents use the same commands, with JSON/TSV output, non-interactive operation, stable exit codes, untrusted-content wrapping, runtime command guards, and baked safety profiles.
- **Preview-first audits.** Drive `tree`, `du`, `inventory`; Contacts `dedupe` previews by default and requires explicit `--apply` for guarded merges; raw API JSON dumps never mutate remote state.
- **Generated reference.** Every command has a docs page produced from `gog schema --json`.

## Pick your path

- **Trying it.** [Install](install.md) → [Quickstart](quickstart.md). Five minutes from `brew install` to your first authenticated query.
- **Wiring up automation.** [Automation](automation.md), [Safety Profiles](safety-profiles.md), and the bundled [`gog` skill](https://github.com/openclaw/gogcli/blob/main/.agents/skills/gog/SKILL.md). Discover the active contract and lock the binary down before handing it to an untrusted caller.
- **Serving MCP tools.** [MCP server](mcp.md) exposes typed, allowlisted tools for agent clients without a generic command bridge.
- **Discovering runtime contracts.** [Automation](automation.md) explains root help, schema metadata, safety controls, and stable exit codes.
- **Polling local events.** [Drive and Docs polling](polling.md) persists cursors and optionally invokes trusted shell hooks.
- **Persisting auth and state.** [Paths and State](paths.md) covers `GOG_HOME`, per-kind directories, XDG paths, and legacy compatibility.
- **Running Workspace at scale.** [Auth Clients](auth-clients.md) for service accounts, named OAuth clients, and domain-wide delegation.
- **Managing Workspace.** [Workspace Admin](workspace-admin.md) covers user creation, cleanup, organizational units, and group administration.
- **Backing up an account.** [Backup](backup.md) before pointing `gog backup push` at a busy mailbox.
- **Selecting private Photos media.** [Photos Picker](photos-picker.md) keeps access limited to items the user explicitly chooses.
- **Managing YouTube.** [YouTube](youtube.md) covers API-key reads, account OAuth, subscriptions, playlists, and mutation safety.
- **Grouping Docs edits atomically.** [Google Docs request batches](docs-batch.md) covers persisted, revision-locked request queues and explicit recovery modes.
- **Verifying real API behavior.** [Live testing](live-testing.md) covers the dedicated-account smoke suite, cleanup, retries, and optional infrastructure.
- **Looking up a flag.** The [Command Index](commands/) has a generated page for every subcommand.

## Project

Active development; the [changelog](https://github.com/openclaw/gogcli/blob/main/CHANGELOG.md) tracks what shipped recently. Goals and non-goals live in the [spec](spec.md). Released under the [MIT license](https://github.com/openclaw/gogcli/blob/main/LICENSE). Not affiliated with Google.
