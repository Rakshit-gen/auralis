-- Notification service schema.
--
-- Idempotency ledger. Every consumed event is recorded here in the same
-- transaction as the writes it drives, so a redelivered event is a no-op even
-- if the service crashed mid-processing last time. Same shape as analytics.
CREATE TABLE processed_events (
    consumer   TEXT NOT NULL,
    event_id   UUID NOT NULL,
    handled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
