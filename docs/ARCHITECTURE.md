# Architecture

Auralis is a serialized-audio streaming platform. This document describes how
the system is put together: the services, how they communicate, where state
lives, and how a request flows through the stack.

## System shape

Eight backend services, one web client, and shared infrastructure (PostgreSQL,
Redis, Kafka, S3-compatible object storage).

```
                         +-------------------+
        browser  ------>  |   web (Next.js)   |
                          +---------+---------+
                                    | HTTPS, JSON
                                    v
                          +-------------------+
                          |   gateway (Go)    |  JWT verify, rate limit,
                          |                   |  identity headers, routing
                          +----+----+----+----+
             REST (identity headers) |  |  |
        +---------------+------------+  |  +-------------------+
        v               v               v                     v
   +---------+     +---------+     +-----------+         +-------------+
   | auth    |     | user    |     | content   |   ...   | ai-media    |
   | (Go)    |     | (Go)    |     | (Go)      |         | (Python)    |
   +----+----+     +----+----+     +-----+-----+         +------+------+
        |               |               |                      |
        |  outbox       |  outbox       |  outbox              | S3 (HLS)
        v               v               v                      v
   +----------------------------------------------+     +-------------+
   |               Apache Kafka                   |     | object store|
   |  auralis.user.events  auralis.content.events |     | MinIO / R2  |
   |  auralis.playback.events  auralis.ai.events  |     +-------------+
   |  auralis.media.events        (+ .dlq)        |
   +----------------------+-----------------------+
                          |
                          v  consumers
             +------------+------------+
             | analytics  | user       | recommendation
             | (Go)       | (Go)       | (Python)
             +------------+------------+
```

## Services

| Service | Language | Owns | Sync deps | Consumes |
| --- | --- | --- | --- | --- |
| `gateway` | Go | nothing (stateless) | all services | none |
| `auth` | Go | users, refresh tokens, login attempts | none | none |
| `user` | Go | profiles, preferences, likes, bookmarks, follows, history, entitlements | none | `user.registered`, content + playback events |
| `content` | Go | shows, seasons, episodes, genres, languages, creators, media pointers, FTS index | none | `media.episode.packaged` |
| `playback` | Go | playback sessions, progress, devices, episode cache | content, user | content publish events |
| `recommendation` | Python | feed projections (show, signal, affinity tables) | none | content + playback events |
| `ai-media` | Python | generation jobs, series bibles, plot threads, episode summaries | content, user | `content.generation.requested` |
| `analytics` | Go | aggregate rollups (daily totals, per-show/episode stats, retention) | none | all domain events |

Each service has its own PostgreSQL database and never reaches into another
service's tables. See [SERVICE_BOUNDARIES.md](SERVICE_BOUNDARIES.md) and
[DATABASE_ARCHITECTURE.md](DATABASE_ARCHITECTURE.md).

## Communication

**Synchronous (REST).** Used when a caller needs an answer now: the gateway
proxying a client request, `playback` asking `content` for episode media keys,
`ai-media` creating a draft show through the `content` internal API. Internal
calls carry the shared service token (`SERVICE_SHARED_TOKEN`) or signed identity
headers, never a user's JWT.

**Asynchronous (Kafka).** Used for everything that can be eventually
consistent: projections, analytics, cross-service reactions. Producers write
the event to an `outbox_events` row in the same database transaction as the
state change, and a relay loop publishes those rows to Kafka. Consumers are
idempotent and track processed event IDs. See [KAFKA.md](KAFKA.md) and
[EVENT_CONTRACTS.md](EVENT_CONTRACTS.md).

## Request lifecycle

A typical authenticated request, `POST /api/playback/progress`:

1. The browser sends the request to the gateway with `Authorization: Bearer <access token>`.
2. The gateway verifies the JWT (HS256, issuer `auralis-auth`, audience `auralis`),
   applies the per-user rate limit, strips any client-supplied identity headers,
   and sets its own: `X-Auralis-User`, `X-Auralis-Roles`, `X-Auralis-Request-Id`,
   and `X-Auralis-Identity-Sig` (HMAC-SHA256 over the first three).
3. The gateway rewrites the path (`/api/playback/progress` becomes `/playback/progress`)
   and reverse-proxies to the `playback` service over a pooled HTTP connection.
4. `playback` trusts the identity headers only after checking the signature,
   writes progress to its database, and enqueues a `playback.progress` event in
   its outbox within the same transaction.
5. The outbox relay publishes the event to `auralis.playback.events`.
6. `analytics`, `user`, and `recommendation` consume it independently and update
   their own projections.

## Storage

- **PostgreSQL**, one logical database per service, all on one instance locally.
- **Redis**, used by the gateway for distributed rate limiting. The gateway
  falls back to an in-process limiter if Redis is unreachable.
- **Object storage** (MinIO locally, Cloudflare R2 in production), holds source
  audio and packaged HLS. Clients stream directly from presigned URLs; audio
  bytes never pass through an application server.

## Media path

`ai-media` (or a creator upload) produces a source WAV, transcodes it to three
AAC bitrates, segments each into a 6-second HLS ladder, writes a master
playlist, and uploads everything to object storage. It then patches the episode
media keys on `content` and emits `media.episode.packaged`. When a listener
hits `POST /api/playback/authorize`, `playback` returns presigned URLs
(2-hour TTL) for the master playlist and each variant. See
[AUDIO_PROCESSING.md](AUDIO_PROCESSING.md).

## Cross-cutting concerns

- **Identity and RBAC**: three roles (`USER`, `CREATOR`, `ADMIN`). The gateway
  authenticates; each service authorizes. See [SECURITY.md](SECURITY.md).
- **Observability**: Prometheus metrics on `/metrics`, OpenTelemetry traces over
  OTLP, structured JSON logs with request and correlation IDs. See
  [OBSERVABILITY.md](OBSERVABILITY.md).
- **Entitlements**: Auralis Premium is a non-payment entitlement granted by a
  promo code or an admin. `playback` enforces it at authorize time.

## Technology choices

Go for the request-path services (gateway, auth, user, content, playback,
analytics) where predictable latency and low memory matter. Python for
`ai-media` and `recommendation` where the work is model orchestration, media
tooling, and numerical ranking. Next.js 16 (App Router, React 19) for the web
client. Rationale for the significant decisions is in
[ARCHITECTURE_DECISIONS.md](ARCHITECTURE_DECISIONS.md).
