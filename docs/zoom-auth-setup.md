# Zoom S2S OAuth Setup

`gog calendar create --with-zoom` and `gog calendar update --with-zoom`
create Zoom meetings through Zoom's Server-to-Server OAuth app type, then attach
the join link to Google Calendar conference data.

## Create the Zoom App

1. Open the Zoom Marketplace.
2. Choose **Develop** > **Build App**.
3. Select **Server-to-Server OAuth**.
4. Name the app for your automation or organization.
5. Copy the app's **Account ID**, **Client ID**, and **Client Secret**.
6. Add the required user-level scopes:
   - `meeting:write`
   - `meeting:read`
   - `user:read`
7. Activate the app after Zoom shows the credentials and scopes are complete.

Do not request `*:admin` scopes for this workflow. Tier 1 Zoom calendar support
creates meetings as the authenticated app account user and does not implement
delegated host selection.

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

Use `--regenerate-zoom` on `gog calendar update` to replace the Zoom meeting, or
`--remove-zoom` to delete the Zoom meeting and clear Calendar conference data.
