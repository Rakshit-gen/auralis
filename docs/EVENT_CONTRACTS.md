# Event Contracts

Every Kafka message is a JSON envelope. The envelope is stable and shared; the
`payload` is specific to the event type and validated by the consumer.

## Envelope

```json
{
  "event_id": "b3b1...-uuid",
  "event_type": "playback.completed",
  "event_version": 1,
  "occurred_at": "2026-09-08T12:34:56.000Z",
  "producer": "playback",
  "correlation_id": "uuid, threads a causal chain across services",
  "causation_id": "uuid of the event or request that caused this one, may be empty",
  "payload": { }
}
```

Required: `event_id` (UUID), `event_type`, `event_version` (>= 1),
`occurred_at`, `producer`, `payload`. `correlation_id` is generated if absent.
An envelope failing validation is dead-lettered without a retry.

## Versioning

`event_version` starts at 1. Rules:

- **Additive change** (new optional payload field): do not bump the version.
  Consumers ignore unknown fields.
- **Breaking change** (rename, remove, retype, change meaning): bump
  `event_version` and have the producer emit both versions until every consumer
  has migrated, then drop the old one.
- Consumers switch on `(event_type, event_version)` and treat an unrecognized
  version as a skip plus a warning metric, never a crash.

## Catalog

### user.registered (v1), producer: auth

```json
{ "user_id": "uuid", "email": "a@b.com", "display_name": "Ada",
  "roles": ["USER"], "registered_at": "2026-09-08T12:00:00Z" }
```
Consumer: user creates the profile, default preferences, base entitlement.

### user.entitlement_changed (v1), producer: user

```json
{ "user_id": "uuid", "entitlement": "premium", "active": true,
  "source": "promo:AURALIS-PREMIUM", "expires_at": "2027-09-08T00:00:00Z" }
```
Consumers: analytics (premium counts), recommendation (preference weighting).

### content.show_published (v1), producer: content

The publication payload is shared by the show and episode publish events. The
base fields describe the show:

```json
{ "show_id": "uuid", "title": "The Vantage", "slug": "the-vantage",
  "language_code": "en", "genre_ids": ["uuid"], "tags": ["thriller"],
  "is_premium": false, "ai_generated": false, "creator_id": "uuid" }
```
Consumers: playback (episode cache), recommendation (`reco_shows`), analytics.

### content.episode_published (v1), producer: content

The show base fields plus the episode identity:

```json
{ "show_id": "uuid", "title": "The Vantage", "slug": "the-vantage",
  "language_code": "en", "genre_ids": ["uuid"], "tags": ["thriller"],
  "ai_generated": false, "creator_id": "uuid",
  "episode_id": "uuid", "episode_number": 3, "episode_title": "Signal Loss",
  "duration_sec": 1180, "is_premium": false }
```

The event carries no media keys. Playback consumes it, then calls
`GET /internal/episodes/{id}` on content to pull the HLS master key and audio
variants into `episode_cache`. Keeping the large, mutable media metadata out of
the event means a re-package does not need a new publish event.

### content.episode_unpublished (v1) / content.episode_archived (v1), producer: content

Same publication payload shape. Playback sets the cache row's `published` flag
to false so new authorizations fail immediately.

### content.audio_uploaded (v1), producer: content

```json
{ "episode_id": "uuid", "show_id": "uuid", "source_key": "uploads/.../source.wav",
  "size_bytes": 48210044, "source": "upload" }
```
Consumer: ai-media enqueues a media packaging job (`kind = "media"`).

### Playback events, producer: playback

Canonical event types (note `playback.completed`, not `playback.complete`):

| event_type | when |
| --- | --- |
| `playback.play` | stream started or resumed |
| `playback.pause` | listener paused |
| `playback.seek` | listener jumped position (`from_sec`, `to_sec`) |
| `playback.skip` | listener skipped forward past content (`from_sec`, `to_sec`) |
| `playback.progress` | periodic heartbeat while listening |
| `playback.completed` | reached the end of the episode |
| `playback.buffer_start` | client stalled waiting for data |
| `playback.buffer_end` | playback resumed after a stall |

Common payload:

```json
{ "user_id": "uuid", "episode_id": "uuid", "show_id": "uuid",
  "event_type": "playback.progress", "position_sec": 615, "duration_sec": 1180,
  "completed": false, "session_id": "uuid", "occurred_at": "2026-09-08T12:00:00Z" }
```

The client sends batches with UPPERCASE type names (`PLAY`, `PAUSE`, `SEEK`,
`SKIP`, `PROGRESS`, `COMPLETE`, `BUFFER_START`, `BUFFER_END`); the playback
service maps them to the canonical domain types above (`COMPLETE` maps to
`playback.completed`). `client_event_id` on the request makes the write
idempotent against retries.

Consumers: analytics (all of them, into rollups), user (`progress` and
`completed` maintain `listening_history`), recommendation (`progress`,
`completed`, `skip` feed `reco_user_show_affinity` and `reco_co_play`).

## Correlation

`correlation_id` is set from the inbound `X-Correlation-Id` (or the request id)
at the gateway and copied onto every envelope a request causes, directly or
through a chain of events. Logs carry the same field, so one id retrieves the
full cross-service trace of an action.
