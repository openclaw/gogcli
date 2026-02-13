---
summary: "Gmail watch + Pub/Sub push in rata"
read_when:
  - Adding Gmail watch/push support
  - Wiring Gmail to downstream webhooks
---

# Gmail watch

Goal: Gmail push → Pub/Sub → `rata` HTTP handler → downstream webhook.

## Quick start

1) Create a Pub/Sub topic (GCP project).
2) Create a push subscription targeting your `rata gmail watch serve` endpoint.
3) Configure push auth:
   - Preferred: OIDC JWT from a service account.
   - Fallback/dev: shared token header `x-gog-token` or `?token=`.
4) Start watch:

```
rata gmail watch start \
  --topic projects/<project>/topics/<topic> \
  --label INBOX
```

5) Run handler:

```
rata gmail watch serve \
  --bind 127.0.0.1 \
  --port 8788 \
  --path /gmail-pubsub \
  --token <shared> \
  --hook-url http://127.0.0.1:18789/hooks/agent
```

## CLI surface

```
rata gmail watch start --topic <gcp-topic> [--label <idOrName>...] [--ttl <sec|duration>]
rata gmail watch status
rata gmail watch renew [--ttl <sec|duration>]
rata gmail watch stop

rata gmail watch serve \
  --bind 127.0.0.1 --port 8788 --path /gmail-pubsub \
  [--verify-oidc] [--oidc-email <svc@...>] [--oidc-audience <aud>] \
  [--token <shared>] \
  [--hook-url <url>] [--hook-token <token>] \
  [--include-body] [--max-bytes <n>] [--save-hook]

rata gmail history --since <historyId> [--max <n>] [--page <token>]
```

Notes:
- `watch start` stores `{historyId, expirationMs, topic, labels}` for account.
- `watch renew` reuses stored topic/labels.
- `watch stop` calls Gmail stop + clears state.
- `watch serve` uses stored hook if `--hook-url` not provided.

## State

Path (per account):

```
~/.config/ratatosk/state/gmail-watch/<account>.json
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

## Error handling

- Stale historyId: fall back to `messages.list` (last N) + reset historyId.
- Watch expired: `watch renew` error; rerun `watch start`.
- Hook failures: log and still advance historyId to avoid replay storms.
