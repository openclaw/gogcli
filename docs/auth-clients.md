# OAuth Clients

Use multiple OAuth client credentials (for different Google Cloud projects or brands) without mixing refresh tokens.

## How it works

- Default client name: `default`
- Default credentials file: `$(os.UserConfigDir())/gogcli/credentials.json`
- Named credentials files: `$(os.UserConfigDir())/gogcli/credentials-<client>.json`
- Tokens are stored per client (`token:<client>:<email>`). Default client also writes legacy keys for backwards compatibility.
- Default account is stored per client, with a legacy global fallback for the default client.

## Selecting a client

Use `--client` (or `GOG_CLIENT`) to pick which credentials + token bucket to use:

```
gog --client work auth credentials ~/Downloads/work-client.json
gog --client work auth add you@company.com
gog --client work gmail search "is:unread"
```

When `--client` is not set, `gog` resolves the client in this order:

1) `--client` / `GOG_CLIENT` override
2) `account_clients` map in config
3) `client_domains` map in config
4) Credentials file named after the email domain (e.g. `credentials-example.com.json`)
5) `default`

## Domain auto-map

To auto-select a client for a domain:

```
gog --client work auth credentials ~/Downloads/work.json --domain example.com
```

This writes `client_domains` into `config.json` so any `@example.com` account selects the `work` client.

## Listing stored credentials

```
gog auth credentials list
```

Shows stored credential files plus any configured domain mappings.

## Config example

```
{
  keyring_backend: "auto",
  account_clients: {
    "you@company.com": "work",
  },
  client_domains: {
    "example.com": "work",
  },
}
```

## Migration notes

- Legacy `token:<email>` entries are copied to `token:default:<email>` the first time they are read.
- Legacy `default_account` is still respected for the default client.
- Browser, manual, remote, and account-manager authorization use S256 PKCE.
  Manual state includes a short-lived verifier under the active `gog` config
  directory. Keep the same `GOG_HOME` and `--client` between remote steps.
- Manual or remote authorization started before v0.24.0 cannot be completed
  after upgrading. Run step 1 again to generate a PKCE-bound URL.

## Workspace service accounts

Workspace Admin, group, org-unit, and Keep automation commonly run through a
service-account key with domain-wide delegation. Store the key for the
Workspace admin identity you want to impersonate:

```
gog auth service-account set admin@example.com --key ~/Downloads/service-account.json
gog auth service-account status admin@example.com
```

Then run Admin SDK commands with that account:

```
gog --account admin@example.com admin users create ada@example.com \
  --first-name Ada \
  --last-name Lovelace \
  --change-password
```

See [Workspace Admin](workspace-admin.md) for user creation, organizational
units, cleanup, and group examples.
