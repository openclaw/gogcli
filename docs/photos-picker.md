---
title: Photos Picker
description: "Let a user select private Google Photos media, then list and download only those chosen items."
---

# Photos Picker

The Photos Picker API is separate from the app-created-only Photos Library API.
It exposes only media the user explicitly selects for a short-lived picking
session.

## Setup

Enable the
[Photos Picker API](https://console.cloud.google.com/apis/library/photospicker.googleapis.com)
in the OAuth client project, then authorize the dedicated service:

```bash
gog auth add you@gmail.com --services photospicker
```

`photospicker` is intentionally excluded from the default `user` and `all-user`
service sets. It uses only
`https://www.googleapis.com/auth/photospicker.mediaitems.readonly`.

## Select Media

Create a session and open its Google-hosted picker:

```bash
gog photos picker create --max-items 20 --open --json
```

The response contains `session.id`, `session.pickerUri`, the expiration time,
and Google's recommended polling configuration. The browser must be signed in
to the same Google account that owns the session.

Wait until the user finishes:

```bash
gog photos picker wait <sessionId> --json
```

`wait` follows the API-provided `pollInterval` and `timeoutIn`. An optional
`--timeout` only shortens that server-provided limit.

## List And Download

List one page or all selected items:

```bash
gog photos picker list <sessionId> --max 100 --json
gog photos picker list <sessionId> --all --json
```

Download an item by its selected-media ID:

```bash
gog photos picker download <sessionId> <mediaItemId> --out photo.jpg
```

Downloads use the authenticated Picker client. Image downloads request the
original media with location metadata removed; video downloads request the
transcoded video bytes and reject videos that are not ready.

Picker `baseUrl` values are short-lived, currently up to 60 minutes, and require
an OAuth bearer token. Prefer the download command instead of persisting or
sharing those URLs.

## Cleanup

Delete the session after retrieving the required bytes:

```bash
gog photos picker delete <sessionId>
```

Google recommends deleting completed or timed-out sessions to avoid session
resource limits. `create` and `delete` support the global `--dry-run` flag.

Command pages:

- [`gog photos picker create`](commands/gog-photos-picker-create.md)
- [`gog photos picker get`](commands/gog-photos-picker-get.md)
- [`gog photos picker wait`](commands/gog-photos-picker-wait.md)
- [`gog photos picker list`](commands/gog-photos-picker-list.md)
- [`gog photos picker download`](commands/gog-photos-picker-download.md)
- [`gog photos picker delete`](commands/gog-photos-picker-delete.md)
