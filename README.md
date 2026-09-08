# Auralis

Auralis is a serialized-audio streaming platform. Listeners follow original
shows told in seasons and episodes, stream them, and pick up where they left
off across devices. Creators build shows, upload or generate episodes, and
watch how they perform. The catalog mixes hand-authored audio with episodes
produced by an asynchronous AI pipeline that writes scripts, synthesises
speech, and packages HLS audio for streaming.

This repository holds the whole system: eight backend services, the web
client, infrastructure manifests, and the tooling to run it locally or deploy
it on free infrastructure.

## Architecture at a glance

Eight independently deployable services, each owning its own database:

| Service | Language | Responsibility |
| --- | --- | --- |
| `gateway` | Go | Edge routing, authentication, rate limiting, request identity |
| `auth` | Go | Registration, login, JWT issuance, refresh token rotation |
| `user` | Go | Profiles, preferences, likes, bookmarks, follows, history, entitlements |
| `content` | Go | Shows, seasons, episodes, genres, Postgres full-text search, publication |
| `playback` | Go | Playback sessions, progress, device state, signed media URLs, playback events |
| `recommendation` | Python | Personalized feed, similar shows, ranking model, offline evaluation |
| `ai-media` | Python | AI story generation, TTS, FFmpeg, HLS packaging, processing jobs |
| `analytics` | Go | Kafka event consumption, aggregate metrics for dashboards |

Services talk to each other over REST for synchronous calls and over Apache
Kafka for asynchronous domain events. Important events are written through an
outbox in the same transaction as the state change and relayed to Kafka
afterwards. Media is stored in S3-compatible object storage (MinIO locally,
Cloudflare R2 in production) and streamed directly from there, never through
an application server.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the full picture and
[`docs/SERVICE_BOUNDARIES.md`](docs/SERVICE_BOUNDARIES.md) for what each
service does and does not own.

## Running locally

Requirements: Docker with Compose v2, and `make`.

```
cp .env.example .env
make up          # start infrastructure and all services
make migrate     # run database migrations for every service
make seed        # load the fictional catalog and synthetic activity
make e2e         # run the critical end-to-end flows against the stack
```

The web client is then on <http://localhost:3000>, the gateway API on
<http://localhost:8080>, Grafana on <http://localhost:3001>, and MinIO on
<http://localhost:9001>.

Full instructions, including running a single service against the shared
infrastructure, are in [`docs/LOCAL_DEVELOPMENT.md`](docs/LOCAL_DEVELOPMENT.md).

## Deployment

The platform runs on free and low-cost tiers: Vercel for the web client, Render
for the services and a single-node Redpanda (Kafka-API compatible), Neon for
Postgres, Upstash for Redis, and Cloudflare R2 for media. It stays functional
with no third-party AI credentials by falling back to the bundled local
generation and TTS providers. See [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).

## Documentation

- [ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [SERVICE_BOUNDARIES.md](docs/SERVICE_BOUNDARIES.md)
- [DATABASE_ARCHITECTURE.md](docs/DATABASE_ARCHITECTURE.md)
- [KAFKA.md](docs/KAFKA.md)
- [EVENT_CONTRACTS.md](docs/EVENT_CONTRACTS.md)
- [PLAYBACK.md](docs/PLAYBACK.md)
- [RECOMMENDATION.md](docs/RECOMMENDATION.md)
- [AI_PIPELINE.md](docs/AI_PIPELINE.md)
- [AUDIO_PROCESSING.md](docs/AUDIO_PROCESSING.md)
- [OBSERVABILITY.md](docs/OBSERVABILITY.md)
- [SECURITY.md](docs/SECURITY.md)
- [DEPLOYMENT.md](docs/DEPLOYMENT.md)
- [LOCAL_DEVELOPMENT.md](docs/LOCAL_DEVELOPMENT.md)
- [TESTING.md](docs/TESTING.md)
- [ARCHITECTURE_DECISIONS.md](docs/ARCHITECTURE_DECISIONS.md)

## License

MIT. See [LICENSE](LICENSE). All shows, episodes, characters, and cover art in
the seed data are original and fictional.
