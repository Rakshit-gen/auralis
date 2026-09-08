# Playback

The playback service authorizes streaming, hands out time-limited media URLs,
tracks progress and playback events, and powers "continue listening" and device
management.

## Authorize

`POST /api/playback/authorize` with `{ "episode_id": "...", "device_id": "..." }`.

Steps:

1. Look up the episode in `episode_cache` (a local projection of publishable
   episodes). A miss triggers a synchronous `GET /internal/episodes/{id}` on
   content and a cache write.
2. Reject if the episode is not published or has no HLS master key.
3. If the episode is premium and the caller is not entitled:
   - if the episode has a free preview window, grant a preview-only session;
   - otherwise return `402` with error code `playback.premium_required`.
4. Presign the HLS master playlist and every audio variant (2-hour TTL).
5. Upsert the device, open a `playback_session`, and read any saved progress.

Response:

```json
{
  "session_id": "uuid",
  "device_id": "uuid",
  "episode": { },
  "hls_master_url": "https://r2/.../master.m3u8?X-Amz-...",
  "variants": [{ "bitrate_kbps": 128, "codec": "aac", "url": "https://r2/..." }],
  "resume_position_sec": 615,
  "preview_only": false,
  "preview_limit_sec": 0,
  "expires_at": "2026-09-08T14:00:00Z"
}
```

The client loads `hls_master_url` with hls.js. Media bytes are served by object
storage, never by the service. When the URL nears expiry the client
re-authorizes.

## Progress

`POST /api/playback/progress` with `{ episode_id, show_id, session_id,
position_sec, duration_sec, client_event_id }` on a short interval (the web
client sends every 15 seconds and on pause). The service:

- upserts `playback_progress` for `(user_id, episode_id)`;
- dedupes on `client_event_id` via `seen_events` so a retried request is a
  no-op;
- enqueues a `playback.progress` event (or `playback.completed` when
  `position_sec` is within the completion threshold of `duration_sec`).

`GET /api/playback/progress/{episodeId}` returns the saved position for resume.

## Events

`POST /api/playback/events` with a batch:

```json
{ "events": [
  { "type": "PLAY", "episode_id": "...", "session_id": "...", "position_sec": 0,
    "client_event_id": "..." },
  { "type": "SEEK", "episode_id": "...", "session_id": "...",
    "from_sec": 120, "to_sec": 300, "client_event_id": "..." }
]}
```

Client types are UPPERCASE (`PLAY`, `PAUSE`, `SEEK`, `SKIP`, `PROGRESS`,
`COMPLETE`, `BUFFER_START`, `BUFFER_END`). The service maps each to a canonical
domain event type (`COMPLETE` maps to `playback.completed`) and enqueues one
outbox row per accepted event, each in its own transaction. The response
reports `accepted` and `skipped` counts. See
[EVENT_CONTRACTS.md](EVENT_CONTRACTS.md) for the taxonomy.

## Continue listening

`GET /api/playback/continue?limit=20` returns in-progress episodes ordered by
`updated_at DESC`, backed by a partial index on `playback_progress` restricted
to rows that are started but not finished. Finished episodes drop off the list.

## Devices

`GET /api/playback/devices` and `PUT /api/playback/devices`
(`{ device_id, name, kind }`). A device is upserted on every authorize so the
list reflects real usage. `kind` defaults to `web`.

## Episode cache

`episode_cache` holds only what authorize needs: media keys, duration, premium
flag, preview window, published flag, show slug and title. It is maintained by
the `auralis.content.events` consumer (`playback-service` group). If it is
wiped, the first authorize for each episode repopulates it from content.

## Entitlement check

Premium enforcement reads the entitlement from the user service. The result is
cached briefly in memory per user id to keep the authorize path fast under a
burst from one listener. A user whose premium lapses loses access to premium
episodes on their next authorize.

## Failure behavior

- Content unreachable on a cache miss: authorize returns `502`.
- Object storage unreachable: authorize returns `502` (no partial URL set).
- User service unreachable during the premium check: authorize returns an
  error rather than granting access, so a premium episode is never handed out
  unverified.
