# Google Auth Platform

`gog auth-platform` contains operator commands for Google Cloud Auth Platform
configuration that is not exposed through Google Discovery APIs.

## OAuth Beta/Test Users

Use `gog auth-platform testers` to list, add, or remove OAuth beta/test users
for a Google Cloud project's OAuth consent screen:

```bash
gog auth-platform testers list --project arc-forge-console --json
gog auth-platform testers add --project arc-forge-console --email user@example.com --json
gog auth-platform testers remove --project arc-forge-console --email user@example.com --force --json
```

The add/remove commands read the current tester list first, write the full
updated list, then verify by reading the result returned by Google.

Required project permissions:

- `resourcemanager.projects.get`
- `oauthconfig.testusers.get`
- `oauthconfig.testusers.update`

`roles/oauthconfig.editor` includes the OAuthConfig permissions. A custom role
with only the permissions above is preferable for automation.

Implementation note: Google documents the IAM permissions but does not currently
publish this tester-list surface through Discovery. The command uses the same
Auth Platform backend shape as Google Cloud Console and keeps that access behind
one small client so it can be replaced if Google publishes a supported public
endpoint.
