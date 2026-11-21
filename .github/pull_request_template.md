<!--
Optional linked context: add a visible `Closes #<issue-number>` or
`Related: #<issue-number>` line below this comment.

Write for someone who uses gog but has not read the code. The title and opening
problem statement should make sense without the diff or linked issue.

Use the repository’s Conventional Commit title style. Name the affected task
and the symptom or outcome, not the implementation.
Illustrative wording only; these examples are not claims of current bugs:
Good: fix: keep JSON output readable by scripts when a warning occurs
Bad: fix(output): move diagnostic writes to a different stream
Good: docs: explain how to authorize Gmail for sending only
Bad: docs(auth): document the gmail-scope enum

For internal-only work, name the actual developer or operator workflow affected.
-->

## What Problem This Solves

<!--
Describe the concrete user, product, or operational problem.
In 1–3 plain sentences, say who is affected, what happens today, and why it
matters to them. For a bug, name the trigger if known. For a feature or internal
work, describe the need in the actual workflow without inventing harm.
Could a reader understand the problem without knowing your proposed fix?

Good: "Background checks can use up a new user's model allowance before they've
asked the assistant to monitor anything."
Bad: "Turn off periodic checks on fresh installs."
-->

## User Impact

<!--
Lead with the concrete outcome in plain language: what users, operators, or
developers can now do or expect, usually in one sentence.
For internal-only changes, say there is no user-visible change; do not invent a benefit.
Keep important risks, breaking changes, migrations, and required user actions visible.
Mention new flags and changes to output, account routing, or permissions when relevant.
-->

## Why This Change Was Made

<!--
Briefly explain how the change addresses the problem without repeating the impact.
Keep the body short. Leave file lists, internal acronyms, and root-cause walkthroughs
in the diff or optional <details>; include technical detail only when it explains
behavior or a material tradeoff. Do not hide risks or required actions in <details>.
-->

## Evidence

<!--
Start with the behavior checked and the result in plain language. Reviewers should
not have to decode command lists to learn what was verified. Focused tests, CLI
output, CI results, and redacted live observations are useful evidence.

Distinguish local fixtures from checks against a real Google account. Note meaningful
gaps and tests not run; do not imply live verification from a mocked response.
Keep the useful summary visible. Put exact commands, commit hashes, and long output
in links or optional <details>. Do not include OAuth credentials, tokens, or private
mail, calendar, or document contents in evidence.
-->
