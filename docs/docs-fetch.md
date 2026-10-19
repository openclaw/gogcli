# Resumable Docs snapshots

```sh
gog docs fetch --ids-file docs.txt --output-dir snapshots --json
gog docs fetch --ids-file docs.txt --output-dir snapshots --refresh --json
```

Put one document ID or Google Docs URL per line; blank lines and duplicate IDs are ignored. Use `--ids-file -` for stdin. Each successful GET requests all tab content and atomically saves `<documentId>.json`. On Unix, files use mode 0600 and newly created directories use mode 0700. On Windows, access follows the output directory's inherited ACLs; select a directory private to your user.

Snapshots contain source URL, account, OAuth client, document title, revision, fetch time, and every tab's ID, title, and plain text, including nested tabs. The command prints a compact per-document result instead of document bodies. It does not perform text analysis.

By default, valid successful snapshots for the same document/account/client are reused without a network request. These are historical snapshots, not freshness or current-access checks: use `--refresh` to fetch current content. Corrupt or identity-mismatched snapshots are refetched. A failed refresh does not erase a previous snapshot; the result still reports the refresh failure.

Individual fetch failures do not stop the remaining list. The final result reports failures and returns nonzero; rerunning the same list reuses successes and retries missing snapshots. `--dry-run` validates IDs and prints the plan without authentication or local/remote writes. Treat snapshot directories as private document content and exclude them from version control.
