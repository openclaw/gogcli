---
title: Workspace Admin
description: "Create, inspect, suspend, delete, and list Google Workspace users, organizational units, and groups from the CLI."
---

# Workspace Admin

`gog admin` uses the Admin SDK Directory API for Workspace user, organizational
unit, and group automation. It is Workspace-only: personal `gmail.com` accounts
cannot use these commands.

Admin commands require an account with Admin SDK access. For unattended use,
configure a service-account key with domain-wide delegation and impersonate a
Workspace admin:

```bash
gog auth service-account set admin@example.com --key ~/Downloads/service-account.json
gog auth service-account status admin@example.com
```

The service account must be delegated the Admin SDK scopes listed by:

```bash
gog auth services --json
```

Organizational-unit commands additionally require the
`https://www.googleapis.com/auth/admin.directory.orgunit` scope in domain-wide
delegation.

## Create Users

Create a user with an explicit initial password:

```bash
gog --account admin@example.com admin users create ada@example.com \
  --first-name Ada \
  --last-name Lovelace \
  --password 'TempPass123!' \
  --change-password \
  --ou /Engineering
```

If `--password` is omitted, `gog` generates a strong temporary password, forces
password change at first login, and prints the generated value in the command
output:

```bash
gog --account admin@example.com admin users create grace@example.com \
  --given Grace \
  --family Hopper \
  --json
```

Create users in restricted states or with recovery metadata:

```bash
gog --account admin@example.com admin users create temp@example.com \
  --given Temp \
  --family User \
  --suspended \
  --recovery-email helpdesk@example.com \
  --recovery-phone +15551234567
```

For pre-hashed passwords, pass the hash and its format:

```bash
gog --account admin@example.com admin users create import@example.com \
  --given Imported \
  --family User \
  --password '<sha1-hash>' \
  --hash-function SHA-1
```

Supported hash functions are `MD5`, `SHA-1`, and `crypt`.

## Inspect And Clean Up

List users in a domain:

```bash
gog --account admin@example.com admin users list --domain example.com --json
```

Get one user:

```bash
gog --account admin@example.com admin users get ada@example.com --json
```

Suspend a user:

```bash
gog --account admin@example.com admin users suspend ada@example.com --force
```

Delete a user:

```bash
gog --account admin@example.com admin users delete ada@example.com --force
```

Use `--dry-run` before create/suspend/delete operations when scripting:

```bash
gog --account admin@example.com admin users create dryrun@example.com \
  --given Dry \
  --family Run \
  --dry-run \
  --json
```

## Organizational Units

List organizational units:

```bash
gog --account admin@example.com admin orgunits list --type all --json
```

Copy-pasted paths from Google or `list` output can include a leading slash;
`get`, `update`, and `delete` accept either form:

```bash
gog --account admin@example.com admin orgunits get /Engineering --json
```

Create a child organizational unit:

```bash
gog --account admin@example.com admin orgunits create Engineering \
  --parent / \
  --description "Engineering users"
```

Rename or update metadata:

```bash
gog --account admin@example.com admin orgunits update /Engineering \
  --name Eng \
  --description ""
```

Delete an empty organizational unit:

```bash
gog --account admin@example.com admin orgunits delete Eng --force
```

## Groups

Group commands share the same Admin SDK setup:

```bash
gog --account admin@example.com admin groups list --domain example.com
gog --account admin@example.com admin groups members add eng@example.com ada@example.com
gog --account admin@example.com admin groups members remove eng@example.com ada@example.com --force
```
