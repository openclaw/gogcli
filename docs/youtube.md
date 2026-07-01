---
title: YouTube
description: "Read YouTube data and manage subscriptions and playlists with gog."
---

# YouTube

`gog youtube` (alias `gog yt`) reads public YouTube data with an API key or
uses account OAuth for private reads and mutations.

## Configure access

For public channel, video, activity, playlist, comment, and search reads, enable
YouTube Data API v3 and store an API key:

```bash
gog config set youtube_api_key YOUR_API_KEY
gog yt videos list --chart mostPopular --region US --max 5
```

Account reads use the default `youtube.readonly` scope:

```bash
gog auth add you@gmail.com --services youtube
gog yt activities list --mine --account you@gmail.com
```

Subscription and playlist mutations require the explicit
`youtube.force-ssl` extra scope:

```bash
gog auth add you@gmail.com --services youtube \
  --extra-scopes https://www.googleapis.com/auth/youtube.force-ssl \
  --force-consent
```

The account must already have a YouTube channel. If the API returns
`youtubeSignupRequired`, initialize the channel once at
[youtube.com](https://www.youtube.com/) and retry.

## Read playlists and liked videos

List the playlists owned by a channel or by the authenticated user:

```bash
gog yt playlists list --channel-id UC_x5XG1OV2P6uZZ5FSM9Ttw
gog yt playlists list --mine --account you@gmail.com
```

List the videos inside a playlist with `playlists items list`. Public playlists
work with an API key; private playlists and the special `LL` (liked videos)
playlist need account OAuth. Use `--all` to page through large playlists:

```bash
gog yt playlists items list --playlist-id PLAYLIST_ID --all
gog yt playlists items list --playlist-id LL --account you@gmail.com --all
```

Read your liked (or disliked) videos directly with `videos list --my-rating`.
This is a per-user read, so it always uses account OAuth:

```bash
gog yt videos list --my-rating like --account you@gmail.com --max 50
gog yt videos list --my-rating dislike --account you@gmail.com --json
```

Both reads work with the default `youtube.readonly` scope; no extra scope is
required.

## Choose video fields

`videos list` keeps its compact historical default: `snippet`,
`contentDetails`, and `statistics`. Request every field available for arbitrary
videos with `--parts all`, or pass an exact comma-separated list:

```bash
gog yt videos list --id VIDEO_ID --parts all --json
gog yt videos list --id YOUR_VIDEO_ID --parts snippet,fileDetails --account you@gmail.com --json
```

`all` excludes the owner-only `fileDetails`, `processingDetails`, and
`suggestions` parts. Explicit lists are passed through unchanged so authenticated
owners can request those fields for their own uploads. Do not combine `all` with
other part names.

## Manage subscriptions

List one page or fetch every page:

```bash
gog yt subscriptions list --max 50 --account you@gmail.com
gog yt subscriptions list --all --account you@gmail.com --json
```

Subscribe with a channel ID:

```bash
gog yt subscriptions subscribe \
  --channel-id UC_x5XG1OV2P6uZZ5FSM9Ttw \
  --account you@gmail.com
```

Unsubscribe using either the subscription ID returned by `subscriptions list`
or a channel ID. Channel-ID removal performs the subscription lookup for you:

```bash
gog yt subscriptions unsubscribe --id SUBSCRIPTION_ID \
  --account you@gmail.com --force
gog yt subscriptions unsubscribe --channel-id UC_x5XG1OV2P6uZZ5FSM9Ttw \
  --account you@gmail.com --force
```

## Manage playlists

New playlists default to private. Set `--privacy unlisted` or
`--privacy public` only when broader visibility is intended.

```bash
gog yt playlists create --title "Research" \
  --description "Videos to review" \
  --account you@gmail.com --json

gog yt playlists add --playlist-id PLAYLIST_ID --video-id VIDEO_ID \
  --position 0 --account you@gmail.com
```

Remove a known playlist item directly, or let `gog` find the item by playlist
and video ID:

```bash
gog yt playlists remove --item-id PLAYLIST_ITEM_ID \
  --account you@gmail.com --force
gog yt playlists remove --playlist-id PLAYLIST_ID --video-id VIDEO_ID \
  --account you@gmail.com --force
```

Delete a playlist:

```bash
gog yt playlists delete PLAYLIST_ID --account you@gmail.com --force
```

## Automation and safety

Every subscription and playlist mutation supports `--dry-run`. Dry runs do not
create an API service or make a network request:

```bash
gog yt playlists add --playlist-id PLAYLIST_ID --video-id VIDEO_ID \
  --account you@gmail.com --dry-run --json
```

Unsubscribe, playlist-item removal, and playlist deletion prompt before the
mutation. Use `--force` only after checking the target, or combine
`--no-input --force` in deliberate automation.

Use `--json` for structured output or `--plain` for stable TSV. Human progress,
prompts, and warnings remain on stderr.

See the generated references for
[`youtube subscriptions`](commands/gog-youtube-subscriptions.md) and
[`youtube playlists`](commands/gog-youtube-playlists.md) for every flag.
