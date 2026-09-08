-- Auth service schema: credentials, roles, refresh token families, outbox.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL,
    email_norm    TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    roles         TEXT[] NOT NULL DEFAULT ARRAY['USER'],
    status        TEXT NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'suspended', 'deleted')),
    display_name  TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_norm_key ON users (email_norm);

-- Refresh tokens are stored hashed. A "family" is the chain of tokens issued
-- from one login; reuse of a rotated token revokes the whole family.
CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    family_id   UUID NOT NULL,
    token_hash  TEXT NOT NULL,
    issued_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    replaced_by UUID REFERENCES refresh_tokens (id),
    user_agent  TEXT NOT NULL DEFAULT '',
    client_ip   TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX refresh_tokens_hash_key ON refresh_tokens (token_hash);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);

CREATE TABLE outbox_events (
    id            BIGSERIAL PRIMARY KEY,
    topic         TEXT NOT NULL,
    partition_key TEXT NOT NULL,
    envelope      JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ
);
CREATE INDEX outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;

CREATE TABLE login_attempts (
    id         BIGSERIAL PRIMARY KEY,
    email_norm TEXT NOT NULL,
    client_ip  TEXT NOT NULL,
    successful BOOLEAN NOT NULL,
    at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX login_attempts_lookup ON login_attempts (email_norm, at);
