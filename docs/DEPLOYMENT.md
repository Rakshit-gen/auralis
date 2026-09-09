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
4. Give the bucket a public read domain and set it on playback as
   `S3_PUBLIC_BASE_URL` (no trailing slash). The quickest option is the R2
   managed `https://pub-<hash>.r2.dev` domain; a custom domain works too. This
   is required for playback in a desktop browser: hls.js loads the variant
   playlists and segments relative to the master URL, and the browser drops the
   query string when it does, so presigned child URLs come back unsigned and
   403. With a public prefix the URLs need no signature. See
   [AUDIO_PROCESSING.md](AUDIO_PROCESSING.md#delivery).
5. Add a CORS policy on the bucket (R2 dashboard, Settings, CORS policy) that
   allows `GET` and `HEAD` from the web client origin, for example:

   ```json
   [{ "AllowedOrigins": ["https://auralis-web-topaz.vercel.app"],
      "AllowedMethods": ["GET", "HEAD"],
      "AllowedHeaders": ["*"],
      "MaxAgeSeconds": 3600 }]
   ```

Create the bucket before the services start; the R2 token does not need
bucket-create permission and the services will not create it for you on R2. The
bucket holds only packaged HLS audio, nothing private. Leave `S3_PUBLIC_BASE_URL`
unset for a local or single-origin demo and playback falls back to presigned
URLs (fine for native HLS on iOS Safari, not for hls.js on desktop).

## 4. Kafka (Redpanda Cloud)

Render's free tier has no private services, so the broker is hosted. Create a
**Redpanda Serverless** cluster (free tier), then a user and its ACLs:

```
rpk cloud login
rpk cloud namespace create auralis
# create the cluster in the console, then:
rpk security user create auralis-app -p '<generated>' --mechanism SCRAM-SHA-256
```

Serverless does not grant anything by default, and each resource type is a
separate ACL. Grant all four before creating topics or starting a service:

```
rpk security acl create --allow-principal User:auralis-app \
  --operation all --topic '*'
rpk security acl create --allow-principal User:auralis-app \
  --operation all --group '*'
rpk security acl create --allow-principal User:auralis-app \
  --operation all --transactional-id '*'
rpk security acl create --allow-principal User:auralis-app \
  --operation all --cluster
```

The transactional-id ACL is what the outbox producers need (they use Kafka
transactions); the cluster ACL covers `IdempotentWrite` and topic listing.
Without the topic ACL a `CreateTopics` call returns success but creates
nothing, so a missing ACL here looks like the topic script working while every
producer then fails, order matters.

Pre-create the topics (Serverless disables auto-creation), all five plus their
`.dlq` counterparts from [KAFKA.md](KAFKA.md), only after the ACLs are in
place. `scripts/deploy/redpanda-topics.py` does this from the `KAFKA_*` values
in `.env.deploy` (partitions 3, broker-default replication), and is idempotent:

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

Confirm they exist (`rpk topic list`) before deploying, an empty list means the
ACLs were not applied first.

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
are both set to `10000`); Render routes external HTTPS to that port. The
ai-media image also downloads the Piper binary and six voice models at build
time (about 360 MB), so its first build is slower than the others; `PIPER_BIN`
and `PIPER_VOICES_DIR` are baked into the image, no Render env needed. ai-media
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

Import the repo as a project (the live one is `auralis-web` at
`https://auralis-web-topaz.vercel.app`), root directory `frontend`. Env:

```
NEXT_PUBLIC_API_BASE=https://auralis-gateway.onrender.com/api
```

The frontend `Dockerfile` builds a standalone bundle and sets
`NEXT_OUTPUT=standalone` itself. Do not set that variable on Vercel, it turns
off Vercel's own output handling and the deploy 404s. It is only for the
container image.

Once the URL is known, set it in two places:

- `CORS_ALLOWED_ORIGINS` on the gateway (comma-separated if more than one).
- The R2 bucket CORS policy (section 3, step 5), so audio segments load.

## 7. Seed the catalog

From a machine with the repo and the venv, pointed at the gateway:

```
SERVICE_SHARED_TOKEN=<value> \
AUTH_BOOTSTRAP_ADMIN_EMAIL=<value> AUTH_BOOTSTRAP_ADMIN_PASSWORD=<value> \
API_BASE=https://auralis-gateway.onrender.com/api \
.venv/bin/python scripts/seed.py
```

## 8. Package audio for the seed catalog

`scripts/seed.py` attaches packaged-audio metadata but never produces audio, so
a freshly seeded episode 404s on playback. `scripts/deploy/media-backfill.py`
closes that gap: for every published episode it composes an original narration
from the show and episode metadata, synthesizes it with Piper (the same neural
voices the ai-media worker uses) or espeak-ng, packages it to the same
three-bitrate HLS layout the ai-media worker produces, uploads it under the key
the episode already points at, and re-attaches the real metadata.

```
scripts/piper-setup.sh                                        # neural voices, once
.venv/bin/python scripts/deploy/media-backfill.py --dry-run   # plan only
.venv/bin/python scripts/deploy/media-backfill.py             # up to --target-gb
.venv/bin/python scripts/deploy/media-backfill.py --force     # re-voice everything
```

It needs `ffmpeg` and `ffprobe` on `PATH` plus either Piper (run
`scripts/piper-setup.sh`, which drops the binary and models in `.piper/`) or
`espeak-ng`. It reads `S3_*` and
gateway settings from `.env.deploy`, is idempotent (an episode that already has
a real master playlist is skipped unless `--force`), and stays inside a byte
budget so the R2 free tier is safe. It runs for a while; `--limit` and
`--only-show` scope a first pass. Episodes generated through the in-app AI
studio already have real audio and are untouched.

## 8b. Cover art for the seed catalog (optional)

The catalogue ships with no real cover art; the web client draws a per-show
gradient instead. `scripts/deploy/cover-backfill.py` replaces that with real
artwork: for every published show it builds a prompt from the title, synopsis
and genres, renders a portrait with a local SDXL + SDXL-Lightning model on
Apple's MPS backend, stores a small WebP under `covers/shows/<id>.webp` in the
media bucket, and PATCHes the show with the public URL and a dominant accent
colour.

```
scripts/img-setup.sh                                          # .imggen venv, once
.imggen/bin/python scripts/deploy/cover-backfill.py --self-test   # no model
.imggen/bin/python scripts/deploy/cover-backfill.py --dry-run     # prompts only
.imggen/bin/python scripts/deploy/cover-backfill.py               # render + attach
```

It needs `S3_PUBLIC_BASE_URL` set (covers are served straight from the public
media domain). The first render downloads ~7 GB of weights to
`~/.cache/huggingface`; after that it is a few seconds of GPU per image at four
steps. It is idempotent (a show that already has `cover_image_url` is skipped
unless `--force`, and an already-uploaded WebP is reused), and `--limit`,
`--only-show` and `--sleep` scope and pace a run. Shows created through the
in-app AI studio can pass a cover at creation and are otherwise untouched.

## 9. Verify

```
API_BASE=https://auralis-gateway.onrender.com/api .venv/bin/python scripts/e2e.py
```

Then load the Vercel URL, register, open a show, play an episode. If playback
starts on iOS Safari but not on a desktop browser, `S3_PUBLIC_BASE_URL` or the
bucket CORS policy is missing (section 3).

## CI/CD

`.github/workflows/ci.yml` runs four jobs behind a `dorny/paths-filter` gate:
`go` (gofmt, vet, tests against a Postgres service and a single-node
`apache/kafka` KRaft container), `python` (ruff check, ruff format check,
pytest), `frontend` (eslint, then `next build`, then `tsc`, then the Vitest
suite, in that order because the build regenerates the route types the
typecheck reads), and `docker` (builds all nine images, no push). Each filter
also lists `.github/workflows/ci.yml` itself so a change to the workflow
exercises every job.

Wire Render and Vercel to deploy on push to the default branch, or trigger
deploys from the workflow with provider deploy hooks.

## Rollback

Render and Vercel both keep previous deploys and offer one-click rollback.
Database migrations are forward-only; a rollback of code that expects an older
schema needs a compatible migration, so keep migrations additive.
