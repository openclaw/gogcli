---
summary: "Drive-backed export commands (Docs/Slides/Sheets)"
read_when:
  - Adding a new Drive-backed export command
  - Changing `--format` / `--out` behavior
---

# Exports (Drive-backed)

Goal: one implementation for “export Google *Thing* via Drive”.

## Current pattern

- Shared builder: `internal/cmd/export_via_drive.go:newExportViaDriveCmd`
- Shared download: `internal/cmd/drive.go:downloadDriveFile` (handles Drive “native” exports + normal files)

Each service command is a thin wrapper:

- `gog docs export <docId> --format pdf|docx|txt|md|html`
- `gog slides export <presentationId> --format pdf|pptx`
- `gog sheets export <spreadsheetId> --format pdf|xlsx|csv`

## Conventions

- Arg is always the Drive file id (Doc/Sheet/Slides id).
- Type guard: compare `mimeType` and error with `file is not a <KindLabel> (mimeType="...")`.
- `--out` defaults to `$(os.UserConfigDir())/gogcli/drive-downloads/` (via `internal/config:EnsureDriveDownloadsDir`).
- `--out` can be dir or explicit file path (via `internal/cmd/drive_download_helpers.go:resolveDriveDownloadDestPath`).
- Output
  - `--json`: `{ "path": "...", "size": <bytes> }`
  - text: `path\t...` / `size\t...`

## Add a new export command

1) Pick expected Drive mime type + allowed formats.
2) Add a new `newXExportCmd` calling `newExportViaDriveCmd(...)`.
3) Add/extend tests in `internal/cmd/execute_drive_*_test.go` style (fake Drive server).
