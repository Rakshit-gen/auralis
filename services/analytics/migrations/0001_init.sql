-- Analytics service schema: aggregate tables built from the Kafka event stream.
-- Dashboards query these; they never touch raw events.

-- Idempotency ledger. Every consumed event is recorded here in the same
-- transaction as the aggregate updates it drives, so a redelivered event is a
-- no-op even if it crashed mid-processing last time.
CREATE TABLE processed_events (
    consumer   TEXT NOT NULL,
    event_id   UUID NOT NULL,
    handled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);

-- One row per (user, day) that had any listening activity. DAU is a count of
-- rows for a day; MAU counts distinct users over a trailing 30 days.
CREATE TABLE listener_days (
    day     DATE NOT NULL,
    user_id UUID NOT NULL,
    PRIMARY KEY (day, user_id)
);
CREATE INDEX listener_days_user_idx ON listener_days (user_id, day);

-- Daily rollup of platform-wide listening.
CREATE TABLE daily_totals (
    day             DATE PRIMARY KEY,
    listening_seconds BIGINT NOT NULL DEFAULT 0,
    plays           BIGINT NOT NULL DEFAULT 0,
    completes       BIGINT NOT NULL DEFAULT 0,
    skips           BIGINT NOT NULL DEFAULT 0,
    buffer_events   BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE episode_stats (
    episode_id        UUID PRIMARY KEY,
    show_id           UUID NOT NULL,
    plays             BIGINT NOT NULL DEFAULT 0,
    completes         BIGINT NOT NULL DEFAULT 0,
    skips             BIGINT NOT NULL DEFAULT 0,
    listening_seconds BIGINT NOT NULL DEFAULT 0,
    -- running sum of per-play completion ratio and its count, for an average
    completion_ratio_sum NUMERIC NOT NULL DEFAULT 0,
    completion_ratio_n   BIGINT NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX episode_stats_show_idx ON episode_stats (show_id);

CREATE TABLE show_stats (
    show_id           UUID PRIMARY KEY,
    title             TEXT NOT NULL DEFAULT '',
    plays             BIGINT NOT NULL DEFAULT 0,
    completes         BIGINT NOT NULL DEFAULT 0,
    listening_seconds BIGINT NOT NULL DEFAULT 0,
    likes             BIGINT NOT NULL DEFAULT 0,
    bookmarks         BIGINT NOT NULL DEFAULT 0,
    follows           BIGINT NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Distinct listeners per show, for the unique-listener metric.
CREATE TABLE show_listeners (
    show_id UUID NOT NULL,
    user_id UUID NOT NULL,
    first_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (show_id, user_id)
);

-- Weekly retention: which users from a signup-week cohort listened in a later
-- week. cohort_week and activity_week are ISO week start dates (Monday).
CREATE TABLE retention (
    cohort_week   DATE NOT NULL,
    activity_week DATE NOT NULL,
    user_id       UUID NOT NULL,
    PRIMARY KEY (cohort_week, activity_week, user_id)
);

CREATE TABLE user_cohorts (
    user_id     UUID PRIMARY KEY,
    cohort_week DATE NOT NULL
);
