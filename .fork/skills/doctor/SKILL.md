---
name: doctor
description: Audit this downstream fork of steipete/gogcli for drift — missing tags, stale patches, broken symlinks, missing workflows, DESIGN_CONFLICT markers left in tracked files. Use when the repo feels off, after a rough sync, or as a periodic sanity check. Does not fix automatically; reports what's wrong and points at the remediation.
---

# doctor

On-demand audit of this repo. Use when something feels off — a sync left strange state, a symlink looks broken, `.fork/patches/` doesn't match `git log`, or you're opening the repo on a new machine and want to sanity-check before committing.

## What this skill does

Reports on the items in `.fork/references/doctor-checklist.md`. For each item, emits one of:

- `ok` — no action needed.
- `fixable` — describes the fix. Safe to apply.
- `broken` — needs a human decision. Describes what's wrong; does not auto-fix.

## Run it

Walk through `.fork/references/doctor-checklist.md` top to bottom. The checklist groups are:

- Layout (directories and symlinks)
- Branches (`main`, `upstream`)
- Tags (`upstream-sync/*`, `sync/*-merged`, `release/*`)
- Patches (`.fork/patches/` vs `git log`)
- Workflows and Mergify (GitHub-side config)
- Snapshots (`.fork/snapshots/*.json`)
- Hygiene (gitignore, `DESIGN_CONFLICT:` markers, smoke test)

For each group, run the relevant commands and compare against what the checklist says should be true.

## Quick checks

Fast ones worth running first:

```
# patches in sync with commits?
diff <(ls .fork/patches/ | sort) <(git format-patch upstream..main --grep='Fork-Patch:' --stdout -o /tmp/fork-check && ls /tmp/fork-check | sort)

# any DESIGN_CONFLICT markers leaked onto main?
git grep 'DESIGN_CONFLICT:' main -- .

# is .fork/.llm-cache tracked?
git ls-files .fork/.llm-cache/ | head

# smoke test
go test ./...
```

## Tools involved

- `.fork/tools/check-upstream.sh` — lightweight check that upstream HEAD matches `.fork/revision.txt`.
- `.fork/tools/export-patches.sh` — if `.fork/patches/` is stale, regenerate here.

## References

- `.fork/references/doctor-checklist.md` — the full list (this skill walks it).
- `.fork/references/architecture.md` — background on why each item matters.
