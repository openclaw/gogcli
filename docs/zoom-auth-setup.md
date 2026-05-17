# Zoom S2S OAuth Setup

`gog calendar create --with-zoom` and `gog calendar update --with-zoom` create
Zoom meetings through Zoom's Server-to-Server OAuth app type, then attach the
join URL, meeting ID, and passcode to the Calendar event description.

> **Note on rendering.** The Zoom info is written into the event description,
> not Calendar's native `conferenceData` surface. Google's Calendar API rejects
> `conferenceData` writes that assert `conferenceSolution.key.type="addOn"` from
> non-Workspace-Marketplace OAuth clients (400 "Invalid conference data") and
> silently drops the field entirely when `key.type` is omitted. Description
> mode renders as a clickable Zoom link in every Calendar UI; the trade-off is
> no native "Join with Zoom" conference card. Becoming a registered
> Workspace Marketplace Conference Add-on is the only path to the native card.

## Prerequisites

`gog` must already be set up against your Google account (`gog auth login`).
See the gogcli quickstart for how to provision a Google OAuth client and seed
`credentials.json` before running `gog auth login`.

## Create the Zoom App

1. Open the Zoom Marketplace.
2. Choose **Develop** > **Build App**.
3. Select **Server-to-Server OAuth**.
4. Name the app for your automation or organization.
5. Copy the app's **Account ID**, **Client ID**, and **Client Secret**.
6. Add the required scopes. User-level scopes (`meeting:write`, `meeting:read`,
   `user:read`) are sufficient on accounts where they are exposed; on accounts
   where the Marketplace UI exposes only granular admin variants, use:
   - `meeting:write:meeting:admin`
   - `meeting:read:meeting:admin`
   - `user:read:user:admin`
7. Activate the app after Zoom shows the credentials and scopes are complete.

Do not request `*:admin` scopes for delegated host selection on this workflow.
Tier 1 Zoom calendar support creates meetings as the authenticated app account
user and does not implement `--zoom-host` delegation.

## Store Credentials

Run:

```bash
gog zoom auth setup
```

The setup command prompts for the account ID, client ID, and client secret. The
client secret is read with masked terminal input, stored in gogcli's existing OS
keyring, and validated with Zoom before it is saved.

Non-secret metadata is written to:

```text
~/.config/gogcli/zoom/default.json
```

The directory is created with `0700` permissions and the metadata file is written
with `0600` permissions. Secrets use namespaced keyring entries:

```text
zoom-account/default/client-secret
zoom-account/default/access-token
```

## Environment Overrides

For CI or ephemeral automation, you can skip stored credentials and set:

```bash
export GOG_ZOOM_ACCOUNT_ID=...
export GOG_ZOOM_CLIENT_ID=...
export GOG_ZOOM_CLIENT_SECRET=...
```

Prefer `gog zoom auth setup` for long-lived machines. Environment variables can
be visible to other processes running as the same user on some systems, so avoid
putting `GOG_ZOOM_CLIENT_SECRET` in shared shell profiles or service logs.

## Verify

Run:

```bash
gog zoom auth doctor
```

The doctor command checks stored or environment credentials, validates them with
Zoom, reports the cached access-token expiry when present, and warns when
`GOG_ZOOM_CLIENT_SECRET` is set.

## Create a Calendar Event With Zoom

```bash
gog calendar create primary \
  --summary "Client sync" \
  --from "2026-05-06T11:00:00+02:00" \
  --to "2026-05-06T11:30:00+02:00" \
  --with-zoom
```

This creates a Zoom meeting via the Zoom API and appends a block like the
following to the event description:

```text
<!-- gog-zoom-meeting:86823956608 -->
Join Zoom Meeting: https://us06web.zoom.us/j/86823956608?pwd=...
Meeting ID: 86823956608
Passcode: <passcode>
<!-- /gog-zoom-meeting -->
```

The HTML comment markers let `--regenerate-zoom` replace the block in place and
`--remove-zoom` strip it out without disturbing surrounding description content.

Command output redacts the managed block's `pwd=` value and `Passcode:` line by
default. Use `--include-passwords` on create/update output only when you really
need to print the password; `gog calendar raw` stays lossless for raw API
inspection.

Use `--regenerate-zoom` on `gog calendar update` to replace the Zoom meeting, or
`--remove-zoom` to delete the Zoom meeting and strip the Zoom block from the
Calendar event description.
