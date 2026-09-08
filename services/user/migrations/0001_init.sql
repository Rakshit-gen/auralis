-- User service schema: profiles, preferences, social graph, history, entitlements.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE profiles (
    user_id      UUID PRIMARY KEY,
    display_name TEXT NOT NULL,
    avatar_url   TEXT NOT NULL DEFAULT '',
    bio          TEXT NOT NULL DEFAULT '',
    email        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE preferences (
    user_id         UUID PRIMARY KEY REFERENCES profiles (user_id) ON DELETE CASCADE,
    genre_slugs     TEXT[] NOT NULL DEFAULT '{}',
    language_codes  TEXT[] NOT NULL DEFAULT '{}',
    autoplay        BOOLEAN NOT NULL DEFAULT true,
    playback_speed  NUMERIC(3,2) NOT NULL DEFAULT 1.0
                    CHECK (playback_speed BETWEEN 0.5 AND 3.0),
    explicit_ok     BOOLEAN NOT NULL DEFAULT true,
    email_updates   BOOLEAN NOT NULL DEFAULT true,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Likes cover both shows and episodes; exactly one target column is set.
CREATE TABLE likes (
    user_id    UUID NOT NULL REFERENCES profiles (user_id) ON DELETE CASCADE,
    target_type TEXT NOT NULL CHECK (target_type IN ('show', 'episode')),
    target_id  UUID NOT NULL,
    show_id    UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, target_type, target_id)
);
CREATE INDEX likes_user_idx ON likes (user_id, created_at DESC);

CREATE TABLE bookmarks (
    user_id    UUID NOT NULL REFERENCES profiles (user_id) ON DELETE CASCADE,
    episode_id UUID NOT NULL,
    show_id    UUID NOT NULL,
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, episode_id)
);
CREATE INDEX bookmarks_user_idx ON bookmarks (user_id, created_at DESC);

CREATE TABLE follows (
    user_id    UUID NOT NULL REFERENCES profiles (user_id) ON DELETE CASCADE,
    show_id    UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, show_id)
);
CREATE INDEX follows_show_idx ON follows (show_id);

-- Listening history is an append-mostly activity log fed by playback events.
-- One row per (user, episode); replays update the last_at and totals.
CREATE TABLE listening_history (
    user_id      UUID NOT NULL REFERENCES profiles (user_id) ON DELETE CASCADE,
    episode_id   UUID NOT NULL,
    show_id      UUID NOT NULL,
    listened_sec INT NOT NULL DEFAULT 0,
    completed    BOOLEAN NOT NULL DEFAULT false,
    first_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, episode_id)
);
CREATE INDEX history_user_idx ON listening_history (user_id, last_at DESC);

-- Entitlements grant access to premium content. For the zero-cost deployment
-- there is no payment provider: a user redeems a promo code, or an admin grants
-- a plan directly. This is deliberately not a simulated payment system.
CREATE TABLE entitlements (
    user_id    UUID PRIMARY KEY REFERENCES profiles (user_id) ON DELETE CASCADE,
    plan       TEXT NOT NULL DEFAULT 'free' CHECK (plan IN ('free', 'premium')),
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'expired', 'revoked')),
    source     TEXT NOT NULL DEFAULT 'default',   -- default, promo_code, admin_grant
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE promo_codes (
    code        TEXT PRIMARY KEY,
    plan        TEXT NOT NULL DEFAULT 'premium',
    duration_days INT NOT NULL DEFAULT 30,
    max_redemptions INT NOT NULL DEFAULT 1000,
    redeemed_count  INT NOT NULL DEFAULT 0,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE promo_redemptions (
    code       TEXT NOT NULL REFERENCES promo_codes (code),
    user_id    UUID NOT NULL REFERENCES profiles (user_id) ON DELETE CASCADE,
    redeemed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (code, user_id)
);

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

INSERT INTO promo_codes (code, plan, duration_days, max_redemptions)
VALUES ('AURALIS-PREMIUM', 'premium', 365, 100000);
