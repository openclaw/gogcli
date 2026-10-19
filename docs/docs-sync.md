# Synchronize Markdown with managed Docs tabs

Create a manifest whose file paths are relative to the manifest:

```json
{"tabs":[{"file":"overview.md","tab":"t.example"},{"file":"design.md","tab":"Design"}]}
```

```sh
gog docs sync DOC_ID --manifest manifest.json --json
gog docs sync DOC_ID --manifest manifest.json --apply --json
```

The default is a live preview. Each entry explicitly associates one nonempty Markdown file with a tab ID or exact title. Duplicate targets and ambiguous titles are rejected. Missing tabs fail unless `--create-tabs` is supplied. The command never deletes, renames, or reorders tabs, and does not replace the whole document. The document URL and existing tab IDs stay stable.

Only managed tabs are rendered using the existing Markdown writer and open-comment orphan checks. The document keeps its existing page layout. Apply may require Drive comment-read permission as well as Docs write permission.

Successful writes save source and rendered-tab hashes in a private local state file, defaulting to `manifest.json.state.json`; override with `--state`. State is tied to the document, account, and OAuth client. Unchanged source/remote pairs skip writes. Remote edits since the last sync stop the entire plan unless explicitly accepted with `--overwrite-remote`. Unknown or invalid state is not silently reset.

State is saved after each successful tab so a partial run can resume. Rendering can require multiple Google requests; it is not an atomic multi-tab transaction. A failure may leave earlier writes completed or the current tab partly updated. Inspect the affected tab before retrying. A prewrite reread detects drift but does not lock editors out during rendering. Preview and `--dry-run` do not modify the document or state file. Keep the state file out of version control.

Temporary provider image `contentUri` URLs are excluded from fingerprints; source URLs, image properties, and formatting remain part of conflict detection. Existing complete state retains its format. State written by an earlier build for an image-bearing tab may report a conflict once because its old hash included temporary URLs; review that tab before explicitly accepting it with `--overwrite-remote`. Incomplete state files are rejected rather than treated as an initial sync.
