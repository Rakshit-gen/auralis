# Deployment

The platform deploys to free tiers of managed providers. Nothing here needs a
paid plan to stand up, though the free tiers sleep idle services and cap
resources.

## Target topology

| Component | Provider | Free-tier notes |
| --- | --- | --- |
| Web client (Next.js) | Vercel | Hobby plan |
| 8 services | Render | Free web services sleep after 15 min idle |
| Kafka | Redpanda Cloud (Serverless) | SASL/SCRAM over TLS; or a single-node Redpanda container |
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
   `analytics`, `ai_media`, `recommendation`. `scripts/deploy/neon-setup.sh`
   does this over `psql` from `NEON_ADMIN_URL` (the direct, non-pooler endpoint);
   it is idempotent.
3. Note the pooled connection string for each. Each service gets exactly one as
   its `*_DATABASE_URL`, pointed at the pooler host with `?sslmode=require`.

## 2. Redis (Upstash)

Create a Redis database, copy the `rediss://` URL. Only the gateway needs it
(`REDIS_URL`); without it the gateway falls back to an in-process limiter.

## 3. Object storage (Cloudflare R2)

1. Create a bucket, for example `auralis-media`.
2. Create an R2 API token (Object Read and Write).
3. Config for ai-media, content, and playback:
   - `S3_ENDPOINT=<account>.r2.cloudflarestorage.com` (host only, no scheme)
   - `S3_USE_SSL=true`
   - `S3_REGION=auto`
   - `S3_BUCKET=auralis-media`
   - `S3_ACCESS_KEY` / `S3_SECRET_KEY` (the R2 token's key pair)

Keep the bucket private. Media is reached only through presigned URLs. Create
the bucket before the services start; the R2 token does not need bucket-create
permission but the services will not create it for you on R2.

## 4. Kafka (Redpanda Cloud)

Render's free tier has no private services, so the broker is hosted. Create a
**Redpanda Serverless** cluster (free tier), then a user and an ACL:

```
rpk cloud login
rpk cloud namespace create auralis
# create the cluster in the console, then:
rpk security user create auralis-app -p '<generated>' --mechanism SCRAM-SHA-256
rpk security acl create --allow-principal User:auralis-app \
  --operation all --topic '*' --group '*'
```

Pre-create the topics (Serverless disables auto-creation), all five plus their
`.dlq` counterparts from [KAFKA.md](KAFKA.md).
`scripts/deploy/redpanda-topics.py` does this from the `KAFKA_*` values in
`.env.deploy` (partitions 3, broker-default replication), and is idempotent:

```
.venv/bin/python scripts/deploy/redpanda-topics.py
```

Or with `rpk`:

```
for t in auralis.user.events auralis.content.events auralis.playback.events \
         auralis.ai.events auralis.media.events; do
  rpk topic create "$t" "$t.dlq" -p 3
done
```

Every service then gets:

```
KAFKA_BROKERS=<seed-broker-host>:9092
KAFKA_SASL_MECHANISM=SCRAM-SHA-256
KAFKA_SASL_USERNAME=auralis-app
KAFKA_SASL_PASSWORD=<generated>
KAFKA_TLS_ENABLED=true
```

The Go and Python clients pick these up for every producer, consumer, and DLQ
writer (see [KAFKA.md](KAFKA.md#transport-security)). Leave them unset for a
local plaintext broker.

Alternative: run one `redpandadata/redpanda` container on a host that offers
free private services (Fly.io, Railway) with `--kafka-addr PLAINTEXT://...` and
no SASL. Single node, no replication, fine for a demo.

## 5. Services (Render)

Eight web services. `infra/render.yaml` is a Render Blueprint that declares all
eight, wired to each other and to the managed infra by environment group.
`scripts/deploy/render.py` creates or updates them through the Render API from
`.env.deploy` and is re-runnable (existing services are updated in place):

```
scripts/deploy/render.py                 # create/update all, then deploy
scripts/deploy/render.py --no-deploy     # sync config only
scripts/deploy/render.py --only auralis-gateway
```

| Render service | Dockerfile | Local port | Type |
| --- | --- | --- | --- |
| auralis-gateway | services/gateway/Dockerfile | 8080 | web (public) |
| auralis-auth | services/auth/Dockerfile | 8081 | web |
| auralis-user | services/user/Dockerfile | 8083 | web |
| auralis-content | services/content/Dockerfile | 8082 | web |
| auralis-playback | services/playback/Dockerfile | 8084 | web |
| auralis-analytics | services/analytics/Dockerfile | 8087 | web |
| auralis-ai-media | services/ai_media/Dockerfile | 8085 | web |
| auralis-recommendation | services/recommendation/Dockerfile | 8086 | web |

On Render every service binds `0.0.0.0:10000` (`<NAME>_HTTP_ADDR` and `PORT`
are both set to `10000`); Render routes external HTTPS to that port. ai-media
runs its generation worker in-process (`AI_MEDIA_RUN_WORKER=true`, the default),
so no separate worker service is needed. The Kafka consumers in user, playback,
analytics, recommendation, and ai-media also run in-process (their
`*_RUN_CONSUMER` toggle defaults to true).

Render's free tier has only web services, so the seven internal services are
also deployed as web services. Each still requires a valid identity signature
(`X-Auralis-Identity-Sig`, HMAC keyed by `IDENTITY_SECRET`) on proxied requests
and `SERVICE_SHARED_TOKEN` on `/internal/*`, so a reachable URL is not an open
door, but for a hardened deployment move them behind a private network on a
paid plan. The gateway reaches them by their `onrender.com` hostnames set as
`AUTH_SERVICE_URL`, `USER_SERVICE_URL`, `CONTENT_SERVICE_URL`,
`PLAYBACK_SERVICE_URL`, `ANALYTICS_SERVICE_URL`, `AI_MEDIA_SERVICE_URL`,
`RECOMMENDATION_SERVICE_URL`. Playback also needs `CONTENT_SERVICE_URL` and
`USER_SERVICE_URL`; ai-media needs `CONTENT_SERVICE_URL` (and optionally
`USER_SERVICE_URL`).

Shared env on every service:

```
JWT_SECRET=<generated>
IDENTITY_SECRET=<generated>
SERVICE_SHARED_TOKEN=<generated>
JWT_ISSUER=auralis-auth
JWT_AUDIENCE=auralis
KAFKA_BROKERS=<redpanda-seed-host>:9092
KAFKA_SASL_MECHANISM=SCRAM-SHA-256
KAFKA_SASL_USERNAME=<redpanda-user>
KAFKA_SASL_PASSWORD=<redpanda-secret>
KAFKA_TLS_ENABLED=true
LOG_LEVEL=info
PORT=10000
OTEL_EXPORTER_OTLP_ENDPOINT=<collector endpoint, optional>
```

Per-service: its `<NAME>_HTTP_ADDR` (`0.0.0.0:10000`), its `*_DATABASE_URL`,
and for content, playback, and ai-media the `S3_*` block. The gateway also
takes `REDIS_URL` and, once the web client is up, `CORS_ALLOWED_ORIGINS`. Auth
also takes `AUTH_BOOTSTRAP_ADMIN_EMAIL` / `AUTH_BOOTSTRAP_ADMIN_PASSWORD` /
`AUTH_BOOTSTRAP_ADMIN_NAME`.

### Migrations

The Go services embed their SQL and run it on every boot, tracking
`schema_migrations`; they do not honor `SKIP_MIGRATE`. The Python services run
Alembic on boot unless `SKIP_MIGRATE=true`. Neon has no advisory-lock
contention here because each service owns its own database. Point the
`*_DATABASE_URL` at the Neon pooler host; migrations run fine over it.

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
