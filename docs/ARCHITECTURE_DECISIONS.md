# Architecture Decisions

Short records of the choices that shaped the system, and what they cost.

## 1. Eight services, database-per-service

**Decision.** Split along business capability: gateway, auth, user, content,
playback, recommendation, ai-media, analytics. Each owns a private database and
exposes an API; no service touches another's tables.

**Why.** The capabilities have genuinely different shapes: content is
write-heavy authoring with review workflow, playback is high-QPS reads with
presigning, recommendation is projection-rebuild batch work, ai-media is
long-running jobs. Independent deploys and independent scaling matter more here
than the simplicity of a monolith.

**Cost.** Cross-service reads become API calls or event projections. There is
no cross-service transaction. The outbox pattern and idempotent consumers exist
to pay this cost safely.

## 2. Polyglot: Go and Python

**Decision.** Go for gateway, auth, user, content, playback, analytics. Python
(FastAPI) for ai-media and recommendation.

**Why.** The Go services are latency-sensitive request routers and CRUD with
strict typing and cheap concurrency. The Python services are model-adjacent:
the generation pipeline and the ranking math are far easier to write, read, and
evaluate in Python, and the eval harness uses the scientific-Python idiom.

**Cost.** Two toolchains, two test runners, two ways to do config. Mitigated by
`libs/go-platform` and `libs/py-common` giving both sides the same envelope
format, health endpoints, metric names, and logging fields.

## 3. Kafka for async, REST for sync

**Decision.** State changes that other services care about are published as
versioned events to Kafka. Anything that needs an answer now is a REST call.

**Why.** Analytics and recommendation are pure consumers that must be able to
rebuild their world from scratch by replaying the log from offset zero. A
queue that drops delivered messages cannot do that. Per-entity ordering (hash
partitioning by aggregate id) and consumer groups are also load-bearing.

**Cost.** Operational weight of a broker. At-least-once delivery forces every
consumer to be idempotent (`processed_events` table). ~30 to 40 second
consumer-group join latency on Kafka 4.x with the Go client is a known
annoyance in tests.

## 4. Transactional outbox, not dual-write

**Decision.** A service writes its state change and the event to publish in the
same database transaction (`outbox_events`). A relay loop publishes unsent rows
on a 1-second tick and marks them sent.

**Why.** Publishing to Kafka inside the request path means either a lost event
(publish fails after commit) or a phantom event (publish succeeds, commit
fails). The outbox makes the event and the state atomic.

**Cost.** Up to ~1 second of publish latency. A polling loop per service.
`FOR UPDATE SKIP LOCKED` keeps it safe to run on every replica.

## 5. Media never transits a service

**Decision.** Audio is packaged to HLS once by the ai-media worker, uploaded to
object storage, and thereafter served only via presigned URLs (2-hour TTL)
minted at authorize time.

**Why.** Streaming bytes through Go handlers would dominate cost and latency
and make autoscaling about bandwidth instead of logic. Presigned URLs push all
of that to the object store's CDN edge.

**Cost.** The client must re-authorize when a URL nears expiry. Preview windows
for premium content need a separate short clip rather than a byte-range trick.

## 6. Providers behind interfaces, local-first

**Decision.** `LLMProvider` and `TTSProvider` are interfaces. Text generation
defaults to a fully local, deterministic procedural generator; Groq is an
optional upgrade selected by env. Speech defaults to Piper neural voices, which
are free and offline and ship in the service image; espeak-ng is the fallback
when the Piper binary or voice models are absent.

**Why.** The platform has to build, run, seed, test, and demo with zero API
keys and zero cost. A hosted model is an enhancement, never a dependency. Any
Groq failure falls back to local mid-job. Piper gives listenable narration
without that trade because it runs on the box; espeak-ng only has to be
intelligible enough for a CI smoke test or a machine with no models fetched.

**Cost.** The local text generator's output is templated fiction, coherent but
not surprising. The Piper voice models add roughly 360 MB to the ai-media image.

## 7. Transparent linear recommender

**Decision.** Ranking is a weighted sum of normalized, named features. Weights
are constants (env-overridable), returned per-feature in the API response.

**Why.** It is explainable, debuggable, evaluable, and has no training
pipeline or model artifact to store and version. The offline harness
(leave-last-out, precision/recall/NDCG, coverage, diversity) measures it
honestly.

**Cost.** Lower intra-list diversity than a learned model would give; the
greedy diversity re-rank compensates. Ceiling on ranking quality.

## 8. Redpanda on Render for the public deployment, not Upstash Kafka

**Decision.** For the free-tier public deployment, run a single-node Redpanda
(Kafka-API compatible) as a private Render service. Vercel for the web client,
Render for the eight services plus Redpanda, Neon for the seven logical
Postgres databases, Upstash for Redis, Cloudflare R2 for media.

**Why.** Upstash deprecated Kafka and does not accept new Kafka users; their
direction is QStash. QStash is an HTTP delivery queue with retries and
scheduling. It is not a replayable log, has no consumer groups, and cannot
replay from offset zero, so it cannot back analytics and recommendation as
designed (decision 3). Redpanda speaks the Kafka API, so it is a drop-in with
zero code changes. A managed Kafka such as Confluent Cloud is the paid
alternative.

**Cost.** A single Redpanda node is a single point of failure and has no
replication. Fine for a demo deployment, not for production; production would
use a replicated cluster or managed Kafka. See [DEPLOYMENT.md](DEPLOYMENT.md).

## 9. Signed identity headers from the gateway

**Decision.** The gateway verifies the JWT once at the edge, then forwards
identity as `X-Auralis-*` headers with an HMAC signature. Services trust the
signature, not the raw headers.

**Why.** Services should not each re-verify JWTs, and they must not trust
client-supplied identity headers. One verification point, one cheap signature
check downstream.

**Cost.** A shared `IDENTITY_SECRET` all services must hold. A service exposed
directly without the gateway serves public routes only.

## 10. No payments

**Decision.** Premium entitlement is a boolean set by an admin or the seed
script. No checkout, no card data, no payment provider.

**Why.** The interesting problems here are streaming, generation, and
event-driven consistency. A payment integration would add compliance surface
and a third-party dependency without teaching anything new.

**Cost.** "Upgrade to premium" is not a real flow. Entitlement changes are an
admin action plus a `user.entitlement_changed` event.
