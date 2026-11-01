-- Local projection of follows, built from user.followed / user.unfollowed.
-- The user service owns follows; this copy exists so a publish can be fanned
-- out without a synchronous call per episode.
CREATE TABLE show_followers (
    show_id     UUID NOT NULL,
    user_id     UUID NOT NULL,
    followed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (show_id, user_id)
);
