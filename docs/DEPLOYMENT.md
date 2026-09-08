# Deployment

The platform deploys to free tiers of managed providers. Nothing here needs a
paid plan to stand up, though the free tiers sleep idle services and cap
resources.

## Target topology

| Component | Provider | Free-tier notes |
| --- | --- | --- |
| Web client (Next.js) | Vercel | Hobby plan |
| 8 services + worker | Render | Free web services sleep after 15 min idle |
| Kafka | single-node Redpanda on Render (private service) | Kafka-API compatible, one container |
| Postgres (7 logical DBs) | Neon | One project, seven databases |
| Redis | Upstash | Free tier, TLS |
| Object storage | Cloudflare R2 | 10 GB-month, 1M Class A ops, 10M Class B ops, free egress |

Upstash Kafka is **not** used: Upstash deprecated it and does not accept new
Kafka users. Redpanda gives the Kafka API with no code change. QStash is not a
substitute because the design needs a replayable log with consumer groups (see
[ARCHITECTURE_DECISIONS.md](ARCHITECTURE_DECISIONS.md), decision 8).

## Prerequisites

Accounts on Vercel, Render, Neon, Upstash, and Cloudflare. The deployer
supplies every credential; none are in the repo.

## 1. Postgres (Neon)

1. Create a Neon project.
2. Create seven databases: `auth`, `users`, `content`, `playback`,
   `analytics`, `ai_media`, `recommendation`.
3. Note the pooled connection string for each. Each service gets exactly one as
   its `*_DATABASE_URL`.

## 2. Redis (Upstash)

Create a Redis database, copy the `rediss://` URL. Only the gateway needs it
(`GATEWAY_REDIS_URL`); without it the gateway falls back to an in-process
limiter.

## 3. Object storage (Cloudflare R2)

1. Create a bucket, for example `auralis-media`.
2. Create an R2 API token (Object Read and Write).
3. Config for ai-media and playback:
   - `S3_ENDPOINT=https://<account>.r2.cloudflarestorage.com`
   - `S3_REGION=auto`
   - `S3_BUCKET=auralis-media`
   - `S3_ACCESS_KEY_ID` / `S3_SECRET_ACCESS_KEY`
   - `S3_FORCE_PATH_STYLE=true`

Keep the bucket private. Media is reached only through presigned URLs.

## 4. Kafka (Redpanda on Render)

Deploy `redpandadata/redpanda` as a **private service** (no public port) with a
single node:

```
redpanda start --overprovisioned --smp 1 --memory 512M --reserve-memory 0M \
  --node-id 0 --check=false \
  --kafka-addr PLAINTEXT://0.0.0.0:9092 \
  --advertise-kafka-addr PLAINTEXT://<render-private-hostname>:9092
```

Attach a small persistent disk at `/var/lib/redpanda/data`. Every service gets
`KAFKA_BROKERS=<render-private-hostname>:9092`. Topics auto-create on first use;
to pre-create them, run `rpk topic create` for the six topics and their `.dlq`
counterparts listed in [KAFKA.md](KAFKA.md).

A single node has no replication. Acceptable for a demo, not for production;
production uses a replicated Redpanda cluster or managed Kafka.

## 5. Services (Render)

Eight web services plus one background worker, each from its Dockerfile in the
repo (`services/<name>/Dockerfile`). Set the build context to the repo root.

| Render service | Dockerfile | Port | Extra |
| --- | --- | --- | --- |
| auralis-gateway | services/gateway/Dockerfile | 8080 | public |
| auralis-auth | services/auth/Dockerfile | 8081 | private |
| auralis-user | services/user/Dockerfile | 8083 | private |
| auralis-content | services/content/Dockerfile | 8082 | private |
| auralis-playback | services/playback/Dockerfile | 8084 | private |
| auralis-analytics | services/analytics/Dockerfile | 8087 | private |
| auralis-ai-media | services/ai_media/Dockerfile | 8085 | private |
| auralis-recommendation | services/recommendation/Dockerfile | 8086 | private |
| auralis-ai-media-worker | services/ai_media/Dockerfile | n/a | `command: worker`, background worker |

Only the gateway is public. It reaches the others by their Render private
hostnames, set as `AUTH_SERVICE_URL`, `USER_SERVICE_URL`, `CONTENT_SERVICE_URL`,
`PLAYBACK_SERVICE_URL`, `ANALYTICS_SERVICE_URL`, `AI_MEDIA_SERVICE_URL`,
`RECOMMENDATION_SERVICE_URL`.

Shared env on every service:

```
JWT_SECRET=<generated>
IDENTITY_SECRET=<generated>
SERVICE_SHARED_TOKEN=<generated>
KAFKA_BROKERS=<redpanda-private-host>:9092
LOG_LEVEL=info
OTEL_EXPORTER_OTLP_ENDPOINT=<collector endpoint, optional>
```

Per-service: its `*_DATABASE_URL`, and for ai-media/playback the `S3_*` block.
Auth also takes `ADMIN_EMAIL` / `ADMIN_PASSWORD` for the bootstrap admin.

### Migrations

Each service runs its own migrations on boot unless `SKIP_MIGRATE=true`. First
deploy: let them run. The Go services embed SQL and track `schema_migrations`;
the Python services run Alembic.

### The free-tier sleep

Render free web services sleep after 15 minutes idle and take ~30 seconds to
wake. The gateway waking is visible as a slow first request. The Kafka
consumers in user, playback, analytics, recommendation, and ai-media mean those
should ideally be paid instances (always on) or the platform accepts consumer
lag until traffic wakes them. For a demo this is acceptable; document it for
whoever clicks the link.

## 6. Web client (Vercel)

Import the repo, root directory `frontend`. Env:

```
NEXT_PUBLIC_API_BASE=https://auralis-gateway.onrender.com/api
```

Set `CORS_ALLOWED_ORIGINS` on the gateway to the Vercel URL.

## 7. Seed the catalog

From a machine with the repo and the venv, pointed at the gateway:

```
SERVICE_SHARED_TOKEN=<value> \
AUTH_BOOTSTRAP_ADMIN_EMAIL=<value> AUTH_BOOTSTRAP_ADMIN_PASSWORD=<value> \
API_BASE=https://auralis-gateway.onrender.com/api \
.venv/bin/python scripts/seed.py
```

## 8. Verify

```
API_BASE=https://auralis-gateway.onrender.com/api .venv/bin/python scripts/e2e.py
```

Then load the Vercel URL, register, open a show, play an episode.

## CI/CD

`.github/workflows` builds and tests Go, Python, and the frontend on every push
with path filters, and builds the Docker images. Wire Render and Vercel to
deploy on push to the default branch, or trigger deploys from the workflow with
provider deploy hooks.

## Rollback

Render and Vercel both keep previous deploys and offer one-click rollback.
Database migrations are forward-only; a rollback of code that expects an older
schema needs a compatible migration, so keep migrations additive.
