---
name: resolve-conflict
description: Resolve rebase conflicts during a sync of this downstream fork of steipete/gogcli. Use when a sync PR has been labeled needs-human because the LLM resolver emitted a DESIGN_CONFLICT marker, or when running a manual sync that hits conflicts. Describes the per-file resolution loop and when to hand off to a human.
---

# resolve-conflict

Use when the LLM resolver bailed (DESIGN_CONFLICT marker) or when running a manual sync that hit a conflict the tool can't close.

The same loop runs in CI (`.github/workflows/fork-conflict-resolve.yml`). This skill documents it for humans and for agents running the loop manually.

## The loop

For each conflicted file:

1. **Read the conflict.** Open the file. Note the `<<<<<<<` / `=======` / `>>>>>>>` markers. Read the commit message of the conflicting patch (`git log --format=%B -1 HEAD`) for the `Fork-Patch:` slug and `Reason:`.

2. **Classify the state.** See `.fork/references/patch-vocabulary.md`. Pick one: `PATCH_CHANGED`, `SRC_CHANGED`, `PATCH_REMOVED`, `SRC_REMOVED`, `NO_PATCH_INFO`, `PATCH_INFO_OUTDATED`. Each state has a prescribed action.

3. **Call the resolver.**
   ```
   .fork/tools/llm_resolve.py --file <path> --state <state>
   ```
   The script loads `.fork/references/resolver-prompt.md`, the fork contract from `.fork/AGENTS.md`, and the commit's trailers. It writes either a resolved file or a file with a `DESIGN_CONFLICT: <reason>` marker inline.

4. **Run the smoke test.**
   ```
   go test ./...
   ```
   If it fails, roll back the resolver's output (`git checkout -- <path>`) and either try a different prompt, fall back to manual editing, or escalate with `DESIGN_CONFLICT:`.

5. **Commit or escalate.**
   - Clean resolution: `git add <path> && git rebase --continue`.
   - `DESIGN_CONFLICT:` present: leave the marker in place, `git add <path> && git rebase --continue`. The sync workflow will copy the marker into the PR body and label `needs-human`.

## Manual DESIGN_CONFLICT handling

When a sync PR is labeled `needs-human`:

1. Check out the sync branch: `git fetch origin && git checkout sync/<date>-<sha>`.
2. Grep for markers: `git grep 'DESIGN_CONFLICT:'`.
3. For each marker, read the `<reason>` after the colon. That's the resolver's best statement of what it couldn't decide.
4. Make the call. Usually one of:
   - The patch is obsolete → revert the `Fork-Patch: <slug>` commit on `main`, rebase sync branch again.
   - The patch needs to be reshaped to fit upstream → edit the file, remove the marker, commit.
   - The `Reason:` needs to change → amend the patch commit, rebase sync branch again.
5. Push. CI reruns. Mergify re-evaluates.

## Tools involved

- `.fork/tools/llm_resolve.py` — the provider-agnostic resolver.
- `.fork/tools/export-patches.sh` — run after the rebase succeeds to refresh `.fork/patches/`.

## References

- `.fork/references/resolver-prompt.md` — prompt template + input schema.
- `.fork/references/patch-vocabulary.md` — state classification and prescribed actions.
- `.fork/references/architecture.md` — where this fits in the sync pipeline.
