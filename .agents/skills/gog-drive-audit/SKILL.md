---
name: gog-drive-audit
description: "Read-only Google Drive sharing and permission audits with gog."
---

# Drive audit

Read `../gog/SKILL.md` and `../gog-drive/SKILL.md` first.

Run bounded, read-only inventory commands:

```bash
gog --account user@example.com --readonly drive audit sharing --max 200 --json --wrap-untrusted
gog --account user@example.com --readonly drive audit sharing --internal-domain example.com \
  --max 200 --json --wrap-untrusted
gog --account user@example.com --readonly drive audit user person@example.com --max 200 \
  --json --wrap-untrusted
```

Classify public links, external-domain grants, broad domain grants, stale-looking direct grants,
and ownership anomalies separately. Include file ID, name, owner, permission, and evidence.

Do not change permissions during an audit. If remediation is requested, present a separate,
reviewable plan and use each mutation's `--dry-run` before any approved write.
