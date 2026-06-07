---
summary: "Gmail watch + Pub/Sub delivery in gog"
read_when:
  - Adding Gmail watch/Pub/Sub support
  - Wiring Gmail to downstream webhooks
---

# Gmail watch

Goal: Gmail publishes mailbox notifications to Pub/Sub, then `gog` turns those
notifications into downstream webhook payloads.

Two delivery modes are supported:

- Pull: `gog gmail watch pull` reads a Pub/Sub subscription from the local
  machine. This is the preferred local-agent shape because Google does not need
  an inbound HTTP route to the machine running `gog`.
- Push: `gog gmail watch serve` exposes an HTTP handler for Pub/Sub push. Use it
  when you intentionally operate a reachable HTTPS endpoint. Push and pull share
  the same downstream hook delivery policy.

## Quick start

1) Create a Pub/Sub topic (GCP project).
2) Create a pull subscription for the topic.
3) Start watch:

```
gog gmail watch start \
  --topic projects/<project>/topics/<topic> \
  --label INBOX
```

4) Run pull consumer:

```
gog gmail watch pull \
  --subscription projects/<project>/subscriptions/<subscription> \
  --hook-url http://127.0.0.1:18789/hooks/agent
```

For push delivery instead:

1) Create a push subscription targeting your `gog gmail watch serve` endpoint.
2) Configure push auth:
   - Preferred: OIDC JWT from a service account.
   - Fallback/dev: shared token header `x-gog-token` or `?token=`.
3) Start watch:

```
gog gmail watch start \
  --topic projects/<project>/topics/<topic> \
  --label INBOX
```

4) Run handler:

```
gog gmail watch serve \
  --bind 127.0.0.1 \
  --port 8788 \
  --path /gmail-pubsub \
  --token <shared> \
  --hook-url http://127.0.0.1:18789/hooks/agent
```

## CLI surface

```
gog gmail watch start --topic <gcp-topic> [--label <idOrName>...] [--ttl <sec|duration>]
gog gmail watch status
gog gmail watch renew [--ttl <sec|duration>]
gog gmail watch stop

gog gmail watch serve \
  --bind 127.0.0.1 --port 8788 --path /gmail-pubsub \
  [--verify-oidc] [--oidc-email <svc@...>] [--oidc-audience <aud>] \
  [--token <shared>] \
  [--hook-url <url>] [--hook-token <token>] \
  [--fetch-delay <sec|duration>] \
  [--include-body] [--max-bytes <n>] [--exclude-labels <id,id,...>] \
  [--history-types <type>...] [--save-hook]

gog gmail watch pull \
  --subscription projects/<project>/subscriptions/<subscription> \
  [--hook-url <url>] [--hook-token <token>] \
  [--fetch-delay <sec|duration>] \
  [--include-body] [--max-bytes <n>] [--exclude-labels <id,id,...>] \
  [--history-types <type>...] [--save-hook]

gog gmail history --since <historyId> [--max <n>] [--page <token>]
```

Notes:
- `watch start` stores `{historyId, expirationMs, topic, labels}` for account.
- `watch renew` reuses stored topic/labels.
- `watch stop` calls Gmail stop + clears state.
- `watch serve` and `watch pull` use stored hook config if `--hook-url` is not
  provided.
- `watch pull` needs Google credentials that can consume the Pub/Sub
  subscription.
- `watch serve` needs an HTTP endpoint reachable by Pub/Sub.
- `watch serve` and `watch pull` default `--exclude-labels` to `SPAM,TRASH`; set to an empty string to disable.
- Exclude label IDs are matched exactly (case-sensitive opaque IDs).
- `watch serve --fetch-delay` and `watch pull --fetch-delay` delay Gmail
  history fetch after each notification (default `3s`) to avoid indexing races;
  accepts seconds (`5`) or Go durations (`5s`).
- `watch serve --history-types` and `watch pull --history-types` accept
  `messageAdded`, `messageDeleted`, `labelAdded`, `labelRemoved` (repeatable or
  comma-separated). Default: `messageAdded` (for backward compatibility).
- `watch serve --history-types` and `watch pull --history-types` must include at
  least one non-empty type.

## State

Path (per account):

```
~/.config/gogcli/state/gmail-watch/<account>.json
```

Schema (v1):

```json
{
  "account": "you@gmail.com",
  "topic": "projects/…/topics/…",
  "labels": ["INBOX"],
  "historyId": "12345",
  "expirationMs": 1730000000000,
  "providerExpirationMs": 1730000000000,
  "renewAfterMs": 1730000001000,
  "updatedAtMs": 1730000001000,
  "hook": {
    "url": "http://127.0.0.1:18789/hooks/agent",
    "token": "...",
    "includeBody": false,
    "maxBytes": 20000
  }
}
```

## Payload to hook

```json
{
  "source": "gmail",
  "account": "you@gmail.com",
  "historyId": "...",
  "deletedMessageIds": ["..."],
  "messages": [
    {
      "id": "...",
      "threadId": "...",
      "from": "...",
      "to": "...",
      "subject": "...",
      "date": "...",
      "snippet": "...",
      "body": "...",
      "bodyTruncated": true,
      "labels": ["INBOX"]
    }
  ]
}
```

## include-body / max-bytes

- Default: headers + snippet only.
- `--include-body`: include text/plain body (first matching part).
- `--max-bytes`: hard cap on body bytes (default `20000`).
- If over cap: truncate + set `bodyTruncated=true`.

## Auth (push)

Preferred:
- Pub/Sub push with OIDC JWT.
- Verify JWT audience + email (service account).

Fallback (dev only):
- Shared token via `x-gog-token` header or `?token=`.

## Auth (pull)

Pull delivery does not expose a public HTTP receiver. The local `gog` process
must have Google credentials for:

- Gmail history reads for the watched account. These use the normal stored
  `gog` Gmail OAuth account selected by `--account` / `--client`.
- Pub/Sub subscriber access on the configured subscription. These use the
  Google Cloud client library credential chain, for example Application Default
  Credentials or `GOOGLE_APPLICATION_CREDENTIALS`, not the stored Gmail OAuth
  token. The credential must be able to consume the subscription; granting
  `roles/pubsub.subscriber` on the subscription is the usual least-privilege
  shape.

The downstream hook token is still local to the hook call from `gog` to the
configured `--hook-url`.

## Error handling

- Stale historyId: fall back to `messages.list` (last N) + reset historyId.
- Watch expired: `watch renew` error; rerun `watch start`.
- Pull mode treats invalid Pub/Sub messages as poison messages: log and
  acknowledge them rather than redelivering forever. Wrong-account
  notifications are also terminal in both modes.
- Hook failures are retryable. `gog` records the hook failure status, preserves
  the pre-hook watch cursor, and returns a delivery failure to Pub/Sub. This
  lets Pub/Sub redeliver the notification after the downstream agent or gateway
  comes back.
- This is an intentional reliability change for existing push deployments.
  Older `watch serve` behavior acknowledged hook failures to avoid replay
  storms, but that could silently lose Gmail wakeups when the downstream
  OpenClaw gateway or agent was temporarily down. The supported behavior is now
  delivery-before-cursor-advance for both push and pull: push returns non-2xx on
  hook failure and pull nacks the message.
- Pub/Sub may retry the same notification until the hook succeeds or until the
  subscription's retry/dead-letter policy takes over. Hook receivers should be
  safe to call more than once for the same Gmail history notification.
- This retry policy is intended for normal Gmail notification volumes. If you
  are processing very high mail rates, for example 1000 messages per minute, run
  your own monitoring, alerting, backlog policy, and dead-letter/backpressure
  setup instead of treating the default watcher as a complete queueing platform.
