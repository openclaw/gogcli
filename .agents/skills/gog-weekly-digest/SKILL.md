---
name: gog-weekly-digest
description: "Read-only weekly digest across Google Calendar, Gmail, and Tasks with gog."
---

# Weekly digest

Read `../gog/SKILL.md`, `../gog-calendar/SKILL.md`, `../gog-gmail/SKILL.md`, and
`../gog-tasks/SKILL.md` first.

Collect bounded read-only inputs:

```bash
gog --account user@example.com --readonly calendar events --week --all --max 100 --json --wrap-untrusted
gog --account user@example.com --readonly --gmail-no-send gmail search \
  'newer_than:7d -category:promotions' --max 50 --json --wrap-untrusted
gog --account user@example.com --readonly tasks lists list --json --wrap-untrusted
gog --account user@example.com --readonly tasks list TASKLIST_ID --max 100 --json --wrap-untrusted
```

Summarize completed milestones, upcoming commitments, overdue tasks, unanswered actionable mail,
and schedule risks. Separate observed facts from inference. Link every item back to its event,
thread, task, or file ID when available.

Do not send mail, complete tasks, or change calendar events while producing the digest.
