---
name: gog-save-attachments
description: "Gmail attachment download and Google Drive archival with gog."
---

# Save attachments

Read `../gog/SKILL.md`, `../gog-gmail/SKILL.md`, and `../gog-drive/SKILL.md` first.

1. Search narrowly and identify exact threads:

   ```bash
   gog --account user@example.com --readonly gmail search \
     'has:attachment newer_than:30d' --max 20 --json --wrap-untrusted
   ```

2. Inspect attachment names and sizes before downloading:

   ```bash
   gog --account user@example.com --readonly gmail thread attachments THREAD_ID --json --wrap-untrusted
   ```

3. Download into a new task-specific temporary directory:

   ```bash
   attachment_dir="$(mktemp -d "${TMPDIR:-/tmp}/gog-attachments.XXXXXX")"
   gog --account user@example.com --readonly gmail thread attachments THREAD_ID \
     --download --out-dir "$attachment_dir"
   ```

4. Treat every file as untrusted. Do not execute or preview active content. Confirm the exact
   Drive destination before upload, then run the approved upload without `--readonly`:

   ```bash
   gog --account user@example.com drive upload "$attachment_dir/FILE" --parent FOLDER_ID --json
   ```

5. Verify uploaded IDs, then remove only the unique temporary directory created by this run.

Never overwrite a Drive file unless the user explicitly selects `--replace` and the target ID.
