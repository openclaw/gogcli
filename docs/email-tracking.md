---
summary: "Email open tracking in rata (Gmail + Cloudflare Worker)"
read_when:
  - Adding/changing Gmail email open tracking
  - Deploying the tracking worker (Cloudflare D1)
---

# Email tracking

Goal: track email opens for `rata gmail send` via a tiny tracking pixel served from a Cloudflare Worker.

High-level:
- `rata gmail send --track` injects a 1×1 image URL into the HTML body.
- The Worker receives the request, stores an “open” row in D1, and returns a transparent pixel.
- `rata gmail track opens …` queries the Worker and prints opens.

Privacy note:
- Tracking is inherently sensitive. Treat this as *instrumentation you opt into per email*.
- The Worker stores IP + user-agent and can derive coarse geo (depending on CF headers/config).

## Setup (local)

Create per-account tracking config + keys:

```sh
rata gmail track setup --worker-url https://gog-email-tracker.<acct>.workers.dev
```

This writes a local config file containing:
- `worker_url` (base URL)
- per-account tracking keys are stored in your keychain/keyring (not in the JSON file)

Optional: auto-provision + deploy with wrangler:

```sh
rata gmail track setup --worker-url https://gog-email-tracker.<acct>.workers.dev --deploy
```

Flags:
- `--worker-name`: default `gog-email-tracker-<account>`.
- `--db-name`: default to worker name.
- `--worker-dir`: default `internal/tracking/worker`.

Re-run `rata gmail track setup` any time to re-print the current `TRACKING_KEY` / `ADMIN_KEY` values (it’s idempotent unless you pass explicit `--tracking-key` / `--admin-key`).

## Deploy (Cloudflare Worker + D1)

From repo root:

```sh
cd internal/tracking/worker
pnpm install
```

Provision secrets (use values printed by `rata gmail track setup`):

```sh
pnpm exec wrangler secret put TRACKING_KEY
pnpm exec wrangler secret put ADMIN_KEY
```

Create and migrate D1:

```sh
pnpm exec wrangler d1 create gog-email-tracker
pnpm exec wrangler d1 execute <db> --file schema.sql
```

Update `wrangler.toml` to reference the D1 `database_id`, then deploy:

```sh
pnpm exec wrangler deploy
```

## Send tracked mail

Tracked email constraints:
- Exactly **one** recipient (`--to`; no cc/bcc).
- HTML body required (`--body-html`).

Optional per-recipient sends:

```sh
rata gmail send \
  --to a@example.com,b@example.com \
  --subject "Hello" \
  --body-html "<p>Hi!</p>" \
  --track \
  --track-split
```

`--track-split` sends separate messages per recipient (no CC/BCC; each message has a unique tracking id).

Example:

```sh
rata gmail send \
  --to recipient@example.com \
  --subject "Hello" \
  --body-html "<p>Hi!</p>" \
  --track
```

## Query opens

By tracking id:

```sh
rata gmail track opens <tracking_id>
```

By recipient:

```sh
rata gmail track opens --to recipient@example.com
```

Status:

```sh
rata gmail track status
```

## Troubleshooting

- `required: --worker-url`: run `rata gmail track setup --worker-url …` first (or pass `--worker-url` again).
- `401`/`403` on `/opens`: admin key mismatch; redeploy secrets and re-run `track setup` if needed.
- No opens recorded:
  - ensure the HTML body contains the injected pixel (view “original” in your mail client).
  - some clients block images by default; “open” only happens after images load.
