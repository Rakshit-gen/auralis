# Security

## Authentication

The auth service is the only issuer of credentials.

- Passwords are hashed with bcrypt (cost 12). Plaintext is never logged or
  stored.
- Login is rate limited and every attempt is written to `login_attempts` with
  the outcome and source IP. Repeated failures for one account slow down.
- Access tokens are JWT (HS256), 15-minute TTL, with claims `iss:
  auralis-auth`, `aud: auralis`, `typ: access`, `sub`, `roles`.
- Refresh tokens are opaque, 30-day TTL, stored hashed, and grouped into
  families. Using a refresh token rotates it; presenting an already-rotated
  token invalidates the whole family (reuse detection) and forces re-login.

## Authorization

Three roles: `USER`, `CREATOR`, `ADMIN`. Roles are additive and travel in the
access token.

- The gateway verifies the JWT at the edge. A request with no or invalid token
  reaches public routes only.
- The gateway strips any inbound `X-Auralis-*` identity headers and sets its
  own: `X-Auralis-User`, `X-Auralis-Roles`, `X-Auralis-Request-Id`, and
  `X-Auralis-Identity-Sig`, an HMAC-SHA256 of the identity headers keyed by
  `IDENTITY_SECRET`.
- Each service verifies that signature before trusting the identity. A service
  reached directly (bypassing the gateway) without a valid signature gets no
  identity and serves public routes only.
- Route-level role checks live in each service (for example, `POST
  /internal/authoring/*` requires `CREATOR`, `GET /ai/jobs` requires `ADMIN`).

## Service-to-service calls

Internal endpoints (`/internal/...`) require the shared bearer token
`SERVICE_SHARED_TOKEN` in addition to being unreachable from the public
gateway routes. Used by ai-media to drive content, and by playback to read
entitlements from user.

## Rate limiting

- Gateway: `GATEWAY_RATE_LIMIT` requests/min per client (Redis token bucket,
  falls open if Redis is down), default 240.
- Auth: its own token-bucket limiter, `AUTH_RATE_LIMIT_RPM` (default 120) with
  `AUTH_RATE_LIMIT_BURST` (default 40), keyed by identity or client IP. This is
  deliberately strict; a single-origin load test has to raise it.

## Transport and secrets

- All secrets come from the environment. `.env.example` ships dev-only values
  and says so; every secret must be changed before any public deployment.
- The three shared secrets (`JWT_SECRET`, `IDENTITY_SECRET`,
  `SERVICE_SHARED_TOKEN`) must match across all services.
- In production every hop is HTTPS (Vercel and Render terminate TLS; the
  Postgres, Redis, and Kafka connections use TLS).
- The media bucket holds only packaged HLS audio. It is served either through
  presigned URLs (2-hour TTL, minted at the playback authorize call) or, where
  `S3_PUBLIC_BASE_URL` is set, through a public read-only media domain with a
  CORS policy scoped to the web client's origin. With the public domain, an
  episode's audio is reachable by anyone who holds or can derive its object
  key, so premium gating on that deployment is the authorize check plus the
  client-side preview limit, not a cryptographic barrier on the bytes. A
  deployment that needs hard enforcement should keep `S3_PUBLIC_BASE_URL` unset
  and serve native HLS, or put a signing proxy in front of the bucket. See
  [AUDIO_PROCESSING.md](AUDIO_PROCESSING.md#delivery).

## Content safety

Generation runs against fictional-only prompt templates. The seed catalog and
all generated material are original fiction set in invented places. There is a
repo-wide check (see [TESTING.md](TESTING.md)) that fails the build on
prohibited strings.

## Input handling

- All request bodies are size-limited and schema-validated (Go structs with
  explicit validation, Pydantic models in Python).
- Every database access is parameterized. Full-text search builds
  `websearch_to_tsquery` input, never string-concatenated SQL.
- CORS allows only the origins in `CORS_ALLOWED_ORIGINS`.

## What is out of scope

No payment flow, so no card data is ever handled. Premium entitlement is a
boolean set by an admin or the seed script. There is no file upload from
anonymous users.
