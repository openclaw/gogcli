# Google Auth Platform

`gog auth-platform` contains operator commands for Google Cloud Auth Platform
configuration that is not exposed through Google Discovery APIs.

## OAuth Beta/Test Users

Use `gog auth-platform testers` to list, add, or remove OAuth beta/test users
for a Google Cloud project's OAuth consent screen:

```bash
gog auth-platform testers list --cloud-project my-project --json
gog auth-platform testers add --cloud-project my-project --email user@example.com --json
gog auth-platform testers remove --cloud-project my-project --email user@example.com --force --json
```

The project flag is named `--cloud-project` because `--project` is the existing
global alias for JSON field projection.

The add/remove commands read the current tester list first, write the full
updated list, then verify by reading the result returned by Google.

Required project permissions:

- `resourcemanager.projects.get`
- `oauthconfig.testusers.get`
- `oauthconfig.testusers.update`

`roles/oauthconfig.editor` includes the OAuthConfig permissions. A custom role
with only the permissions above is preferable for automation.

Implementation note: Google documents the IAM permissions but does not currently
publish this tester-list surface through Discovery or as a supported public API.
The command depends on an undocumented Google Cloud Console backend and can stop
working without notice. The implementation is isolated so it can be replaced if
Google publishes a supported endpoint.
