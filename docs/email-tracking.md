---
summary: "Email open tracking in gog (Gmail + Cloudflare Worker)"
read_when:
  - Adding/changing Gmail email open tracking
  - Deploying the tracking worker (Cloudflare D1)
---

# Email tracking

Goal: track email opens for `gog gmail send` via a tiny tracking pixel served from a Cloudflare Worker.

High-level:
- `gog gmail send --track` injects a 1×1 image URL into the HTML body.
- The Worker receives the request, stores an “open” row in D1, and returns a transparent pixel.
- `gog gmail track opens …` queries the Worker and prints opens.

Abuse controls:
- Repeated opens for the same tracking id, IP, and user-agent are deduplicated within a one-hour window.
- Each IP can record at most 100 opens per hour; excess requests still receive the transparent pixel but are not inserted into D1.

Privacy note:
- Tracking is inherently sensitive. Treat this as *instrumentation you opt into per email*.
- The Worker stores recipient email, subject hash, sent/open timestamps, IP, user-agent, bot classification, and coarse geo from Cloudflare request metadata when available.
- The deployed Worker includes a daily cron trigger that deletes open rows older than 90 days.
- Admin `/opens` queries default to 100 rows and are capped at 500 rows per request.

## Setup (local)

Create per-account tracking config + keys:

```sh
gog gmail track setup --worker-url https://gog-email-tracker.<acct>.workers.dev
```

This writes a local config file containing:
- `worker_url` (base URL)
- the active tracking key version
- per-account tracking/admin keys are stored in your keychain/keyring (not in the JSON file)

Optional: auto-provision + deploy with wrangler:

```sh
gog gmail track setup --worker-url https://gog-email-tracker.<acct>.workers.dev --deploy
```

Flags:
- `--worker-name`: default `gog-email-tracker-<account>`.
- `--db-name`: default to worker name.
- `--worker-dir`: default `internal/tracking/worker`.

Re-run `gog gmail track setup` any time to re-print the current `TRACKING_KEY` / `ADMIN_KEY` values (it’s idempotent unless you pass explicit `--tracking-key` / `--admin-key`).

## Deploy (Cloudflare Worker + D1)

From repo root:

```sh
cd internal/tracking/worker
pnpm install
```

Provision secrets (use values printed by `gog gmail track setup`):

```sh
pnpm exec wrangler secret put TRACKING_KEY
pnpm exec wrangler secret put TRACKING_KEY_V1
pnpm exec wrangler secret put TRACKING_CURRENT_KEY_VERSION
pnpm exec wrangler secret put ADMIN_KEY
```

Create D1:

```sh
pnpm exec wrangler d1 create gog-email-tracker
```

Update `wrangler.toml` to reference the D1 `database_id`, then migrate and deploy:

```sh
pnpm exec wrangler d1 execute gog-email-tracker --file schema.sql --remote
pnpm exec wrangler deploy
```

`wrangler.toml` includes a daily cron trigger for retention cleanup. After deploy, Cloudflare calls the Worker once per day and the Worker deletes open rows older than 90 days.

## Rotate tracking keys

Rotate the pixel encryption key without invalidating old tracking ids:

```sh
gog gmail track key rotate
```

The command generates the next key version, deploys all active `TRACKING_KEY_V<N>` secrets plus `TRACKING_CURRENT_KEY_VERSION`, then stores the new current version in local config. Legacy unversioned tracking ids still decrypt through the stored `TRACKING_KEY` fallback.

For local-only testing:

```sh
gog gmail track key rotate --no-deploy
```

Do not send newly tracked mail after `--no-deploy` until the Worker has the matching versioned secret, or new pixels will not decrypt.

## Send tracked mail

Tracked email constraints:
- Exactly **one** recipient (`--to`; no cc/bcc).
- HTML body required (`--body-html`).

Optional per-recipient sends:

```sh
gog gmail send \
  --to a@example.com,b@example.com \
  --subject "Hello" \
  --body-html "<p>Hi!</p>" \
  --track \
  --track-split
```

`--track-split` sends separate messages per recipient (no CC/BCC; each message has a unique tracking id).

Example:

```sh
gog gmail send \
  --to recipient@example.com \
  --subject "Hello" \
  --body-html "<p>Hi!</p>" \
  --track
```

## Query opens

By tracking id:

```sh
gog gmail track opens <tracking_id>
```

By recipient:

```sh
gog gmail track opens --to recipient@example.com
```

Status:

```sh
gog gmail track status
```

## Troubleshooting

- `required: --worker-url`: run `gog gmail track setup --worker-url …` first (or pass `--worker-url` again).
- `401`/`403` on `/opens`: admin key mismatch; redeploy secrets and re-run `track setup` if needed.
- New tracked messages do not show opens after key rotation: verify the Worker has `TRACKING_KEY_V<N>` for the current local `gmail track status` version and `TRACKING_CURRENT_KEY_VERSION` matches it.
- No opens recorded:
  - ensure the HTML body contains the injected pixel (view “original” in your mail client).
  - some clients block images by default; “open” only happens after images load.
