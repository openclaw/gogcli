---
name: gog-contacts-cleanup
description: "Google Contacts duplicate review and guarded cleanup with gog."
---

# Contacts cleanup

Read `../gog/SKILL.md` and `../gog-contacts/SKILL.md` first.

1. Detect duplicates without mutation:

   ```bash
   gog --account user@example.com --readonly contacts dedupe --match email,phone,name --json --wrap-untrusted
   ```

2. Present each proposed group with resource IDs and the fields that matched. Flag conflicts in
   names, organizations, notes, and non-empty phone/email values.
3. Preview the exact merge plan:

   ```bash
   gog --account user@example.com contacts dedupe --resource people/ONE \
     --resource people/TWO --apply --dry-run --json
   ```

4. Apply only explicitly approved groups. Omit `--force` unless the user requested non-interactive
   execution after reviewing the plan.

Never merge solely on a similar name. Prefer exact normalized email or phone evidence and retain
the richest contact as the primary resource.
