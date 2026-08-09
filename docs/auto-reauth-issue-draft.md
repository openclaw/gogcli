# Feature: Auto-reauth on expired/revoked refresh tokens

## Summary

When a stored OAuth refresh token is expired or revoked (`invalid_grant`),
`gog` fails with a hard error and requires the user to manually re-run
`gog auth add`. This is in contrast to other CLI tools (e.g. Rust's
`yup-oauth2` `InstalledFlowAuthenticator`) which automatically fall back to
a browser-based re-authorization flow when the refresh token is invalid,
making the failure transparent to the user.

## Problem

When the refresh token becomes invalid (Google revokes it for various
reasons: 6-month inactivity, password change, token limit reached, app in
Testing mode with 7-day expiry, user manually revoking access), any
`gog` command that needs authentication fails:

```
$ gog calendar events --today
Error: refresh access token: oauth2: "invalid_grant" "Token has been expired or revoked."
```

The user must then manually run:

```
gog auth add you@example.com --services gmail,calendar,drive
```

This is particularly frustrating because:

1. **It's a hard stop** — no graceful degradation or recovery hint beyond the
   raw OAuth error.
2. **The error message doesn't tell the user what to do** — it's a wrapped
   `golang.org/x/oauth2` error, not a user-facing diagnostic.
3. **It happens repeatedly** — especially for users whose OAuth app is in
   Testing mode (7-day refresh token expiry) or who don't use `gog` daily.
4. **Other tools handle this transparently** — `yup-oauth2` (used by the
   `today` Rust CLI) detects `invalid_grant` during token refresh and
   automatically falls back to the Installed Flow (opens a browser, obtains a
   new refresh token, persists it, and retries the original request).

## Proposed behavior

When `gog` detects `invalid_grant` during a token refresh attempt:

1. **If running interactively** (stdin is a terminal, `--no-input` is not set):
   - Print a message to stderr: "Refresh token expired or revoked. Re-authorizing…"
   - Automatically launch the browser-based OAuth flow (same as `gog auth add`)
     using the stored account's services and client.
   - On success, persist the new refresh token to the keyring and retry the
     original API request.
   - On failure (user denies, browser doesn't open, etc.), surface a clear
     error with the re-auth command to run manually.

2. **If running non-interactively** (`--no-input`, CI, pipes):
   - Do NOT auto-launch a browser.
   - Surface a clear error message with the exact `gog auth add` command to run.

## Design considerations

- **Security**: Auto-reauth should only trigger when the refresh token is
  specifically revoked/expired (`invalid_grant`), not for other OAuth errors.
  The browser flow uses the same PKCE + state validation as `gog auth add`.
- **Scope preservation**: The reauth should request the same services/scopes
  as the stored token, not a broader or narrower set.
- `--force-consent` should be used during auto-reauth to ensure Google
  returns a new refresh token (without it, Google may omit the refresh token
  for returning users).
- **Keychain access**: The reauth flow needs keychain write access to persist
  the new token. On macOS, this may trigger a Keychain permission prompt.
- **Timeout**: The auto-reauth browser flow should have a reasonable timeout
  (e.g. 2 minutes) to avoid hanging indefinitely in CI-like environments.

## Prior art

- `yup-o-auth2` (Rust): `InstalledFlowAuthenticator::find_token_info()` —
  on refresh failure, falls back to `auth_flow.token()` which opens a browser.
  Source: [authenticator.rs](https://github.com/dermesser/yup-oauth2/blob/master/src/authenticator.rs)

- gogcli already has partial auth resilience:
  - v0.31.0: Recover from corrupt token payloads (#872)
  - v0.32.0: Retry on 403 insufficient scopes by refreshing credentials (#889)
  - v0.33.0: Trust Developer-ID-signed binaries for Keychain access

  Auto-reauth on `invalid_grant` is the next gap in this progression.

## Environment

- gog: v0.34.0 (Homebrew)
- macOS: 15.x (also reproduced on macOS 27 Tahoe beta)
- Keyring: macOS Keychain (also affects file backend)
