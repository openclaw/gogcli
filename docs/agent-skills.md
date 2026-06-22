---
title: Agent skills
description: "Install service-specific and workflow skills for using gog with coding agents."
---

# Agent skills

`gog` ships concise skills for its Google services plus curated cross-service workflows.
Service skills are generated from `gog schema --json`, so their command lists stay aligned
with the installed CLI. Workflow skills remain hand-authored because task order, safety,
and cleanup require judgment.

## Install

Install the repository's skills with any Agent Skills-compatible client:

```bash
npx skills add https://github.com/openclaw/gogcli
```

For OpenClaw, copy or symlink selected directories from `.agents/skills/` into the
configured skills directory. Start with `gog`, then add only the services and workflows
the agent needs.

## Included workflows

- `gog-inbox-triage`: prioritize unread mail without sending
- `gog-meeting-prep`: combine Calendar context with linked Drive material
- `gog-save-attachments`: download Gmail attachments and optionally upload to Drive
- `gog-drive-audit`: inspect public, external, and user-specific sharing
- `gog-weekly-digest`: summarize Calendar, Gmail, and Tasks
- `gog-contacts-cleanup`: review duplicates before an explicitly confirmed merge

## Regenerate service skills

```bash
make agent-skills
make agent-skills-check
```

Edit the CLI command/help metadata, regenerate, and commit the resulting service-skill
changes together. Do not hand-edit generated service skills.
