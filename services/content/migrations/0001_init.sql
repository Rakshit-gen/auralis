-- Content service schema: catalog, review workflow, media metadata, search.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE languages (
    code TEXT PRIMARY KEY,          -- BCP-47 primary subtag, e.g. en, hi, es
    name TEXT NOT NULL
);

CREATE TABLE genres (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT ''
);

-- A creator profile is created the first time a user with the CREATOR role
-- creates a show. user_id comes from the gateway identity headers.
CREATE TABLE creators (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    bio          TEXT NOT NULL DEFAULT '',
    avatar_url   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Lifecycle shared by shows, seasons, and episodes.
--   draft            editable by the creator
--   ready_for_review submitted, waiting on a reviewer
--   approved         reviewer accepted, not yet public
--   published        publicly listable and streamable
--   rejected         reviewer declined, back to the creator
--   archived         withdrawn from the catalog
CREATE TYPE content_status AS ENUM
    ('draft', 'ready_for_review', 'approved', 'published', 'rejected', 'archived');

-- Episode audio processing state, driven by the ai-media service.
CREATE TYPE processing_status AS ENUM
    ('none', 'queued', 'generating_script', 'synthesizing', 'assembling', 'packaging', 'ready', 'failed');

CREATE TABLE shows (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    creator_id     UUID NOT NULL REFERENCES creators (id),
    title          TEXT NOT NULL,
    slug           TEXT NOT NULL UNIQUE,
    synopsis       TEXT NOT NULL DEFAULT '',
    description    TEXT NOT NULL DEFAULT '',
    language_code  TEXT NOT NULL REFERENCES languages (code),
    genre_ids      UUID[] NOT NULL DEFAULT '{}',
    tags           TEXT[] NOT NULL DEFAULT '{}',
    maturity       TEXT NOT NULL DEFAULT 'general'
                   CHECK (maturity IN ('general', 'teen', 'mature')),
    cover_image_url TEXT NOT NULL DEFAULT '',
    accent_color   TEXT NOT NULL DEFAULT '#c98a3c',
    is_premium     BOOLEAN NOT NULL DEFAULT false,
    status         content_status NOT NULL DEFAULT 'draft',
    ai_generated   BOOLEAN NOT NULL DEFAULT false,
    episode_count  INT NOT NULL DEFAULT 0,
    total_duration_sec BIGINT NOT NULL DEFAULT 0,
    rating_sum     BIGINT NOT NULL DEFAULT 0,
    rating_count   BIGINT NOT NULL DEFAULT 0,
    published_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    search_vector  tsvector
);
CREATE INDEX shows_status_idx ON shows (status);
CREATE INDEX shows_language_idx ON shows (language_code);
CREATE INDEX shows_genres_idx ON shows USING gin (genre_ids);
CREATE INDEX shows_tags_idx ON shows USING gin (tags);
CREATE INDEX shows_published_idx ON shows (published_at DESC) WHERE status = 'published';
CREATE INDEX shows_search_idx ON shows USING gin (search_vector);

CREATE TABLE seasons (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    show_id      UUID NOT NULL REFERENCES shows (id) ON DELETE CASCADE,
    number       INT NOT NULL,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    status       content_status NOT NULL DEFAULT 'draft',
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (show_id, number)
);

CREATE TABLE episodes (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    show_id            UUID NOT NULL REFERENCES shows (id) ON DELETE CASCADE,
    season_id          UUID NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    number             INT NOT NULL,
    title              TEXT NOT NULL,
    slug               TEXT NOT NULL,
    synopsis           TEXT NOT NULL DEFAULT '',
    script             TEXT NOT NULL DEFAULT '',
    status             content_status NOT NULL DEFAULT 'draft',
    processing         processing_status NOT NULL DEFAULT 'none',
    processing_error   TEXT NOT NULL DEFAULT '',
    is_premium         BOOLEAN NOT NULL DEFAULT false,
    free_preview_sec   INT NOT NULL DEFAULT 0,
    ai_generated       BOOLEAN NOT NULL DEFAULT false,
    ai_job_id          UUID,
    duration_sec       INT NOT NULL DEFAULT 0,
    -- media metadata, populated when packaging completes
    hls_master_key     TEXT NOT NULL DEFAULT '',
    audio_variants     JSONB NOT NULL DEFAULT '[]',   -- [{bitrate_kbps, key, codec, size_bytes}]
    codec              TEXT NOT NULL DEFAULT '',
    sample_rate_hz     INT NOT NULL DEFAULT 0,
    channels           INT NOT NULL DEFAULT 0,
    file_size_bytes    BIGINT NOT NULL DEFAULT 0,
    checksum_sha256    TEXT NOT NULL DEFAULT '',
    published_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    search_vector      tsvector,
    UNIQUE (season_id, number)
);
CREATE INDEX episodes_show_idx ON episodes (show_id, number);
CREATE INDEX episodes_status_idx ON episodes (status);
CREATE INDEX episodes_processing_idx ON episodes (processing) WHERE processing NOT IN ('none', 'ready', 'failed');
CREATE INDEX episodes_published_idx ON episodes (published_at DESC) WHERE status = 'published';
CREATE INDEX episodes_search_idx ON episodes USING gin (search_vector);

-- Direct uploads: a pending record is created when a presigned URL is handed
-- out, and confirmed once the client reports the upload finished.
CREATE TABLE media_uploads (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    episode_id   UUID NOT NULL REFERENCES episodes (id) ON DELETE CASCADE,
    object_key   TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes   BIGINT NOT NULL DEFAULT 0,
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'uploaded', 'rejected')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at TIMESTAMPTZ
);

CREATE TABLE review_events (
    id          BIGSERIAL PRIMARY KEY,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('show', 'season', 'episode')),
    entity_id   UUID NOT NULL,
    reviewer_id UUID NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('submit', 'approve', 'reject', 'publish', 'archive')),
    notes       TEXT NOT NULL DEFAULT '',
    from_status content_status,
    to_status   content_status NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX review_events_entity_idx ON review_events (entity_type, entity_id, created_at);

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

-- Full-text search vectors. English config is adequate for the seed catalog;
-- per-language configs can be added later without a schema change.
CREATE FUNCTION shows_search_refresh() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', array_to_string(NEW.tags, ' ')), 'B') ||
        setweight(to_tsvector('english', coalesce(NEW.synopsis, '')), 'C') ||
        setweight(to_tsvector('english', coalesce(NEW.description, '')), 'D');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER shows_search_trg BEFORE INSERT OR UPDATE OF title, tags, synopsis, description
    ON shows FOR EACH ROW EXECUTE FUNCTION shows_search_refresh();

CREATE FUNCTION episodes_search_refresh() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(NEW.synopsis, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER episodes_search_trg BEFORE INSERT OR UPDATE OF title, synopsis
    ON episodes FOR EACH ROW EXECUTE FUNCTION episodes_search_refresh();
