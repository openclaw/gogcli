---
summary: "Reply once to matching Gmail messages with gog"
read_when:
  - Adding Gmail email automation
  - Setting up mailbox auto-replies without Gmail templates
---

# Gmail auto-reply

`gog gmail autoreply` is a one-shot command.

Use it on a schedule with `launchd`, cron, or another runner when Gmail's native filter API is not enough.

## Example

Reply once to mail sent to `security@openclaw.ai`, add a dedupe label, then archive:

```bash
gog -a clawdbot@gmail.com gmail autoreply \
  'to:security@openclaw.ai in:inbox -label:SecurityAutoReplied' \
  --body-file ./security-autoreply.txt \
  --label SecurityAutoReplied \
  --archive \
  --mark-read
```

Recommended reply text:

```text
Thanks for contacting OpenClaw Security.

For fastest triage, please submit security reports privately via GitHub:
https://github.com/openclaw/openclaw/security/advisories/new

If you cannot use GitHub or are unsure which repo is affected, reply to this email.
```

## Notes

- The command is idempotent per thread via the post-reply label.
- It sends replies in-thread with `In-Reply-To` and `References`.
- It adds `Auto-Submitted: auto-replied` and suppresses common auto-response loops.
- By default it skips bulk/list/auto-generated mail.
