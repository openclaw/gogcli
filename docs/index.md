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
- **Multi-account, multi-client.** Many Google accounts and OAuth client projects in one config; OAuth, direct access tokens, ADC, and Workspace service accounts all supported.
- **Built for agents.** Runtime allow/deny lists (`--enable-commands`, `--disable-commands`, `--gmail-no-send`) plus baked safety-profile binaries that cannot be reconfigured at runtime.
- **Read-only audits.** Drive `tree`, `du`, `inventory`; Contacts `dedupe` preview; raw API JSON dumps without ever mutating remote state.
- **Generated reference.** Every command has a docs page produced from `gog schema --json`.

## Pick your path

- **Trying it.** [Install](install.md) → [Quickstart](quickstart.md). Five minutes from `brew install` to your first authenticated query.
- **Wiring up an agent.** [Safety Profiles](safety-profiles.md) and the bundled [`gog` agent skill](https://github.com/steipete/gogcli/blob/main/.agents/skills/gog/SKILL.md). Lock the binary down before handing it to a model.
- **Running Workspace at scale.** [Auth Clients](auth-clients.md) for service accounts, named OAuth clients, and domain-wide delegation.
- **Backing up an account.** [Backup](backup.md) before pointing `gog backup push` at a busy mailbox.
- **Looking up a flag.** The [Command Index](commands/) has a generated page for every subcommand.

## Project

Active development; the [changelog](https://github.com/steipete/gogcli/blob/main/CHANGELOG.md) tracks what shipped recently. Goals and non-goals live in the [spec](spec.md). Released under the [MIT license](https://github.com/steipete/gogcli/blob/main/LICENSE). Not affiliated with Google.
