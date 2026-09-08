# Testing

Four layers: unit and integration tests per service, cross-service end-to-end
flows, an offline recommendation evaluation, and k6 load tests. `make test`
runs the first layer for every language.

## Unit and integration

### Go (`make test-go`)

Each Go module has `go test ./...`. These need a local Postgres and Kafka (or
the Compose stack) because the service tests run against a real database and,
where relevant, a real broker rather than mocks.

| Test | Covers |
| --- | --- |
| `libs/go-platform/authn` | JWT issue/verify, claim and audience checks |
| `libs/go-platform/envelope` | event envelope encode/decode, version rules |
| `libs/go-platform/httpx` | token-bucket rate limiter behavior |
| `services/auth/internal` | register, login, refresh rotation, reuse detection, rate limiting |
| `services/user/internal` | profile, preferences, entitlement, event consumption |
| `services/content/internal` | authoring, the review state machine, FTS, publication events |
| `services/playback/internal` | authorize, premium and preview paths, progress dedupe, event mapping |
| `services/gateway/internal` | JWT edge verification, identity header signing, proxying |
| `services/analytics/internal` | idempotent consumption, projection updates |

### Python (`make test-py`)

`pytest` in each service.

| Test | Covers |
| --- | --- |
| `services/ai_media/tests/test_generation.py` | series and episode pipelines end to end with the local providers, determinism by seed, continuity state |
| `services/recommendation/tests/test_recommend.py` | feature scoring, weight application, diversity re-rank, cold start |

### Frontend (`make test-web`)

Vitest: `src/lib/__tests__/format.test.ts` (duration and date formatting),
`src/stores/__tests__/player.test.ts` (player store transitions).

## End-to-end (`make e2e`)

`scripts/e2e.py` runs against a running stack and asserts cross-service
effects, 23 checks: register and log in, browse the catalog, search, authorize
playback, save progress and resume it, like and follow, kick off an AI series
generation and poll the job, and read analytics as admin. Exits non-zero on the
first failure. This is the smoke test after any deploy.

## Offline recommendation evaluation (`make reco-eval`)

Leave-last-out split over the seed dataset. For each user with at least three
recorded interactions, the most recent is held out and masked, the feed is
recomputed, and the held-out item's rank drives the metrics.

Latest run, 37 users:

| k | precision | recall | NDCG | catalog coverage | mean intra-list diversity |
| --- | --- | --- | --- | --- | --- |
| 5 | 0.1405 | 0.7027 | 0.5529 | 0.8438 | 0.0614 |
| 10 | 0.0838 | 0.8378 | 0.5937 | 0.9375 | 0.0627 |
| 20 | 0.0486 | 0.9730 | 0.6269 | 1.0000 | 0.0675 |

Precision is capped low by there being one relevant held-out item per user
(max precision at k=5 is 0.2). Recall reaching 0.97 by k=20 and NDCG in the
0.55 to 0.63 range are the real signal. See
[RECOMMENDATION.md](RECOMMENDATION.md) for interpretation.

## Load tests (`make load`)

k6, against the gateway. `k6 run -e BASE=https://your-gateway load/<file>.js` to
point at a deployment.

### `load/catalog.js`

Public browse and search. Ramping to 40 VUs, ~1m50s.

Latest local run (native stack, seeded catalog):

| metric | value |
| --- | --- |
| requests | 13,948 at 126.2/s |
| http_req_failed | 0.00% |
| http_req_duration med / p90 / p95 | 1.03 / 1.81 / 2.21 ms |
| http_req_duration max | 49.3 ms |
| catalog_search_ms p95 | 1.65 ms |
| catalog_show_ms p95 | 1.67 ms |
| checks | 13,947 / 13,947 passed |

Thresholds (`http_req_failed < 1%`, `p95 < 400ms`, `p99 < 800ms`, search `p95 <
500ms`) all pass with wide margin. The public read path is served almost
entirely from Postgres with warm caches and never touches Kafka or object
storage.

### `load/playback.js`

Authenticated: each VU registers its own listener, authorizes, and streams six
progress ticks. Ramping to 20 VUs, ~2m20s.

Latest local run (native stack, `AUTH_RATE_LIMIT` raised for the run):

| metric | value |
| --- | --- |
| requests | 3,321 at 23.3/s |
| http_req_failed | 0.45% (15 requests) |
| authorize med / p90 / p95 | 1.27 / 2.13 / 2.66 ms |
| progress med / p90 / p95 | 2.00 / 4.69 / 7.68 ms |
| authorize / progress max | ~60 s (a few requests) |
| checks | 3,273 / 3,288 passed |

Read that carefully: the authorize and progress endpoints themselves are a few
milliseconds at p95. The failures and the ~60-second tail are all in the
`register` call. The auth service hashes passwords with bcrypt cost 12; twenty
virtual users each registering a fresh account from one origin saturates the
CPU on a laptop, and a handful of registrations queue past the 60-second client
timeout. In a real deployment registration is a rare, human-paced event and
each VU here does it on every iteration, so this is a load-shape artifact, not a
playback-path regression. Lowering bcrypt cost or giving auth more CPU removes
the tail; the playback numbers stand on their own.

The gateway upstream connection pool
(`services/gateway/internal/gateway.go`) was sized (512 idle conns, 128 per
host) after an early run exhausted ephemeral ports and drove authorize p95 to
~106ms with near-total failures; after the fix authorize p95 is a few
milliseconds, as above.

The auth service's own rate limiter (`AUTH_RATE_LIMIT_RPM` 120,
`AUTH_RATE_LIMIT_BURST` 40, keyed by IP) also throttles this test because every
VU registers from one origin. That is correct anti-abuse behavior. To measure
the playback path under real load, raise the limit for the run
(`AUTH_RATE_LIMIT_RPM=100000 AUTH_RATE_LIMIT_BURST=5000` on the auth service)
and restore it afterward.

## CI

`.github/workflows` runs, with path filters:

- Go: `gofmt` check, `go vet`, `go test` with a Postgres and Kafka service
  container.
- Python: `ruff check`, `ruff format --check`, `pytest`.
- Frontend: `npm run lint`, `npm run typecheck`, `npm run test`, `npm run
  build`.
- Docker: build every service image.

## Prohibited-content check

A repo-wide grep gate (run in CI and before release) fails on: the em dash
character, unfinished-work markers, unimplemented-function exceptions,
placeholder body text, placeholder API keys, and a list of AI-marketing
phrases. Keeps the codebase and docs free of scaffolding and filler.
