-- Playback service schema: sessions, progress, devices, event idempotency, and
-- a local cache of published episode metadata for authorization.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Populated from content.episode_published / content.show_published events so
-- authorization does not depend on a synchronous call to the content service on
-- every play. A cache miss falls back to a REST lookup.
CREATE TABLE episode_cache (
    episode_id      UUID PRIMARY KEY,
    show_id         UUID NOT NULL,
    show_slug       TEXT NOT NULL DEFAULT '',
    show_title      TEXT NOT NULL DEFAULT '',
    episode_title   TEXT NOT NULL DEFAULT '',
    episode_number  INT NOT NULL DEFAULT 0,
    duration_sec    INT NOT NULL DEFAULT 0,
    is_premium      BOOLEAN NOT NULL DEFAULT false,
    free_preview_sec INT NOT NULL DEFAULT 0,
    hls_master_key  TEXT NOT NULL DEFAULT '',
    audio_variants  JSONB NOT NULL DEFAULT '[]',
    published       BOOLEAN NOT NULL DEFAULT true,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL,
    name         TEXT NOT NULL DEFAULT 'Unknown device',
    kind         TEXT NOT NULL DEFAULT 'web'
                 CHECK (kind IN ('web', 'mobile', 'tablet', 'desktop', 'speaker', 'other')),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, id)
);
CREATE INDEX devices_user_idx ON devices (user_id);

CREATE TABLE playback_sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL,
    episode_id   UUID NOT NULL,
    show_id      UUID NOT NULL,
    device_id    UUID,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at     TIMESTAMPTZ
);
CREATE INDEX sessions_user_idx ON playback_sessions (user_id, last_seen_at DESC);

-- One progress row per (user, episode). position_sec never decreases from an
-- out-of-order event: updates are guarded by last_event_at.
CREATE TABLE playback_progress (
    user_id       UUID NOT NULL,
    episode_id    UUID NOT NULL,
    show_id       UUID NOT NULL,
    position_sec  INT NOT NULL DEFAULT 0,
    duration_sec  INT NOT NULL DEFAULT 0,
    completed     BOOLEAN NOT NULL DEFAULT false,
    last_event_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, episode_id)
);
CREATE INDEX progress_continue_idx ON playback_progress (user_id, updated_at DESC)
    WHERE completed = false AND position_sec > 0;

-- Idempotency for client-generated events. A redelivered event id is ignored.
CREATE TABLE seen_events (
    client_event_id TEXT PRIMARY KEY,
    user_id         UUID NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX seen_events_gc_idx ON seen_events (received_at);

CREATE TABLE outbox_events (
    id            BIGSERIAL PRIMARY KEY,
    topic         TEXT NOT NULL,
    partition_key TEXT NOT NULL,
    envelope      JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;

CREATE TABLE processed_events (
    consumer   TEXT NOT NULL,
    event_id   UUID NOT NULL,
    handled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
