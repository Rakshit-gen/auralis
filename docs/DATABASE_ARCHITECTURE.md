# Database Architecture

## Database per service

Every service has its own PostgreSQL database and connects only to that one.
Locally this is one `postgres:16` instance with seven databases created by
`infra/compose/postgres-init.sql`:

| Database | Service |
| --- | --- |
| `auth` | auth |
| `users` | user |
| `content` | content |
| `playback` | playback |
| `analytics` | analytics |
| `ai_media` | ai-media |
| `recommendation` | recommendation |

In production these are separate managed databases (Neon projects or
branches). No service has credentials for another service's database, and there
are no cross-database queries or foreign keys. This is enforced by
configuration: each service reads exactly one `*_DATABASE_URL`.

## Ownership and projections

State is either **authoritative** (the service is the source of truth) or a
**projection** (a read-optimized copy rebuilt from events).

Authoritative examples: `auth.users`, `content.shows`, `user.entitlements`,
`playback.playback_progress`.

Projection examples: `playback.episode_cache` (from content events),
everything in the `recommendation` database, everything in the `analytics`
database. A projection can be dropped and rebuilt by replaying the relevant
Kafka topics from the beginning.

## Migrations

**Go services** embed ordered `.sql` files under `migrations/` and apply them
with `libs/go-platform/migrate`. Applied versions are recorded in
`schema_migrations`; re-running is a no-op. There are no down migrations;
roll forward with a new file.

```
<service> migrate      # apply pending migrations, then exit
```

**Python services** use Alembic. `ai-media` and `recommendation` each have an
`alembic/` directory with a single initial revision.

```
python -m auralis_ai_media migrate
python -m auralis_reco migrate
```

`make migrate` runs all seven in order.

## Common patterns

**Outbox.** `auth`, `user`, `content`, and `playback` each have an
`outbox_events` table:

```sql
CREATE TABLE outbox_events (
    id            BIGSERIAL PRIMARY KEY,
    topic         TEXT NOT NULL,
    partition_key TEXT NOT NULL,
    envelope      JSONB NOT NULL,      -- the full versioned event envelope
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;
```

The domain write and the `INSERT INTO outbox_events` happen in one transaction.
A relay loop (1-second tick) selects unpublished rows `ORDER BY id LIMIT 500
FOR UPDATE SKIP LOCKED`, publishes the batch to Kafka in one request, and sets
`published_at = now() WHERE id = ANY($ids)`. `SKIP LOCKED` lets multiple service
instances relay concurrently without double-sending within a batch. See
[KAFKA.md](KAFKA.md).

**Processed events.** Every consumer has:

```sql
CREATE TABLE processed_events (
    consumer  TEXT NOT NULL,
    event_id  UUID NOT NULL,
    PRIMARY KEY (consumer, event_id)
);
```

A handler checks this table before doing work and inserts after, so a
redelivered event is a cheap no-op. Handlers are also written to be naturally
idempotent where possible (upserts, not blind inserts).

**Partial indexes for hot paths.** `playback.playback_progress` has a partial
index on `(user_id, updated_at DESC)` restricted to in-progress rows to make
"continue listening" a cheap lookup.

## Full-text search

The `content` database holds the search index inline. `shows` and `episodes`
each have a `tsvector` column maintained by a `BEFORE INSERT OR UPDATE`
trigger. Weights: for shows, A=title, B=tags, C=synopsis, D=description; for
episodes, A=title, C=synopsis. Queries use `websearch_to_tsquery` and rank with
`ts_rank_cd`. There is no external search engine. See [SEARCH in the content
handlers] and the migration for the exact trigger definitions.

## Connection pooling

Go services use `pgxpool` with pool sizes set per service in `main.go`. Python
services use SQLAlchemy async engines with `pool_size` configured in settings.
Under load-test conditions the request-path services were checked for pool
starvation; the pools are sized so the default k6 profile does not exhaust
them.
