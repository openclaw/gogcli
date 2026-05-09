# Drive Audits

read_when:
- Auditing Drive folder contents, size, or inventory without changing files.
- Reviewing `drive tree`, `drive du`, or `drive inventory`.

Drive audit commands are read-only reporting helpers. They are meant for cleanup
planning, migration review, and automation that needs stable JSON without
writing back to Drive.

## Commands

- [`gog drive tree`](commands/gog-drive-tree.md)
- [`gog drive du`](commands/gog-drive-du.md)
- [`gog drive inventory`](commands/gog-drive-inventory.md)
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

`drive du` counts files under folders and sorts by `size`, `path`, or `files`.

## Inventory Export

Export a read-only item inventory:

```bash
gog drive inventory --parent <folderId> --json
gog drive inventory --parent <folderId> --max 0 --depth 0 --json > drive-inventory.json
```

Use inventory output when you need a machine-readable list of Drive objects for
review, diffing, or downstream cleanup scripts.

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
