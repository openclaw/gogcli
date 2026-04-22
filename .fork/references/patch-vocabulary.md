# Patch vocabulary — gogcli

Structured vocabulary `.fork/tools/llm_resolve.py` uses when describing patch drift to the LLM. Borrowed from Brave's `gitPatcher.js`. Every conflict falls into one of these states; the prescribed action tells the resolver what to do.

## States

### `PATCH_CHANGED`
The fork's patch has changed since the last sync; upstream's file is unchanged in the patched region. Safe to re-apply the patch as-is against the new upstream.

**Resolver action**: apply the fork-side diff to `UPSTREAM_AFTER`. Confirm the result matches `FORK_AFTER` (modulo upstream's unrelated changes). Commit.

### `SRC_CHANGED`
Upstream changed the file in or around the patched region; the patch itself is unchanged from the fork's side. This is the common refactor case — upstream renamed, reformatted, or reorganized the code the patch targeted.

**Resolver action**: translate the patch to fit the new upstream shape. Preserve the patch's `Reason:`. Use the modify/delete grep heuristic in `resolver-prompt.md` to find where the patched concept lives now. Commit if a clean translation exists; otherwise `DESIGN_CONFLICT:`.

### `PATCH_REMOVED`
The fork-side patch has been revoked (commit removed or reverted). Nothing to do on the upstream side.

**Resolver action**: accept upstream's version as-is. No patch to apply. This state should be rare — normally revocation is an explicit commit, not a sync-time event.

### `SRC_REMOVED`
Upstream deleted the file (or the entire region) the patch targets. The patch has no valid target anymore.

**Resolver action**: run the modify/delete grep heuristic to check if upstream moved the concept to a new file. If so, translate the patch to the new location. If not, write `DESIGN_CONFLICT: upstream removed <file|region>; patch "<slug>" has no valid target`. Do not silently drop the patch.

### `NO_PATCH_INFO`
No `Fork-Patch:` / `Reason:` trailers on the conflicting commit. Probably a commit from before this skill's conventions existed, or hand-authored outside the tooling.

**Resolver action**: treat as best-effort three-way merge. Use the standard conflict-resolution heuristics without the benefit of intent metadata. Log the missing trailers so the human can backfill them.

### `PATCH_INFO_OUTDATED`
Trailers exist but don't match the current reality — e.g. `Reason:` says "disable telemetry" but the diff touches an unrelated module. The metadata is stale.

**Resolver action**: flag it. Write `DESIGN_CONFLICT: patch "<slug>" trailers disagree with diff; needs human to either update the trailers or split the commit`.

## Using the vocabulary

The resolver receives the state as `PATCH_STATE` in its inputs (see `resolver-prompt.md`). The state drives which rule branch above applies. All six states are legal; the resolver must handle each.

If the state cannot be classified cleanly, the script passes `NO_PATCH_INFO` as a safe default and logs the ambiguity for the human.
