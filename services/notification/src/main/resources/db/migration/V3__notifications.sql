-- One row per notification shown to a user. The unique key on
-- (user_id, kind, episode_id) means an episode that is unpublished and
-- published again does not notify the same follower twice.
CREATE TABLE notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL,
    kind       TEXT NOT NULL,
    show_id    UUID NOT NULL,
    episode_id UUID,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at    TIMESTAMPTZ,
    UNIQUE (user_id, kind, episode_id)
);

-- The inbox query: newest first for one user.
CREATE INDEX notifications_user_created_idx ON notifications (user_id, created_at DESC, id DESC);

-- Unread badge count without scanning read rows.
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;
