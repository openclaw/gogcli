# Drive Audits

read_when:
- Auditing Drive folder contents, size, or inventory without changing files.
- Reviewing `drive tree`, `drive du`, `drive inventory`, or permission audits.

Drive audit commands are read-only reporting helpers. They are meant for cleanup
planning, migration review, and automation that needs stable JSON without
writing back to Drive.

## Commands

- [`gog drive tree`](commands/gog-drive-tree.md)
- [`gog drive du`](commands/gog-drive-du.md)
- [`gog drive inventory`](commands/gog-drive-inventory.md)
- [`gog drive audit sharing`](commands/gog-drive-audit-sharing.md)
- [`gog drive audit user`](commands/gog-drive-audit-user.md)
- [`gog drive ls`](commands/gog-drive-ls.md)
- [`gog drive get`](commands/gog-drive-get.md)
- [`gog drive raw`](commands/gog-drive-raw.md)

## Folder Tree

Print a readable folder tree:

```bash
gog drive tree --parent <folderId> --depth 2
```

Use JSON when another tool should consume the result:

```bash
gog drive tree --parent <folderId> --depth 3 --json
```

## Size Summary

Summarize folder sizes:

```bash
gog drive du --parent <folderId> --max 20
gog drive du --parent <folderId> --depth 2 --sort size --json
```

`drive tree` and `drive inventory` emit one row per discovered placement, so
legacy items reachable through multiple parents retain each path. `drive du`
aggregates those placements independently and sorts by `size`, `path`, or
`files`. Shortcuts count as file placements with zero content bytes; scans do
not follow shortcut targets.

## Inventory Export

Export a read-only item inventory:

```bash
gog drive inventory --parent <folderId> --json
gog drive inventory --parent <folderId> --max 0 --depth 0 --json > drive-inventory.json
```

Use inventory output when you need a machine-readable list of Drive objects for
review, diffing, or downstream cleanup scripts.

## Revisions

List revision metadata for a Drive file, then inspect one revision:

```bash
gog drive revisions list <fileId> --all --json
gog drive revisions get <fileId> <revisionId> --json
```

The Drive API exposes revision IDs, timestamps, keep-forever state, and
provider export links where available. For native Docs Editors files it does
not expose complete editor history or historical document bodies.

Command pages:

- [`gog drive revisions`](commands/gog-drive-revisions.md)
- [`gog drive revisions list`](commands/gog-drive-revisions-list.md)
- [`gog drive revisions get`](commands/gog-drive-revisions-get.md)

## Permission Audits

Find public or external shares without changing files:

```bash
gog drive audit sharing --parent <folderId> --internal-domain example.com --json
gog drive audit sharing --parent <folderId> --public-only --fail-found
```

Find files shared with a specific user:

```bash
gog drive audit user clawdbot@gmail.com --parent <folderId> --json
```

Bulk permission operations are intentionally separate from audits and require a
dry run or confirmation:

```bash
gog drive bulk remove-public --parent <folderId> --dry-run
gog drive bulk update-role --parent <folderId> --from writer --to reader --target contractor@example.com --dry-run
```

Permission paging must finish for every file included in the scan. API failures,
repeated page tokens, and page-limit errors fail without partial audit success
output. Bulk operations complete this permission scan before writing changes, so
a listing failure prevents writes; this does not roll back later write failures.

## Shared Drives

The audit commands include shared drives by default where the underlying Drive
API supports it. Pass `--no-all-drives` to restrict a scan to My Drive:

```bash
gog drive inventory --parent root --no-all-drives --json
```

## Custom Fields

For object-level inspection, use `drive get --fields`:

```bash
gog drive get <fileId> --fields 'id,name,mimeType,size,owners,emailAddress' --json
```

Use [`gog drive raw`](commands/gog-drive-raw.md) when you need the raw Drive API
object, with the sensitive-field behavior described in [Raw API Dumps](raw-api.md).
