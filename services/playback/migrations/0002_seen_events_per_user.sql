-- Client event ids are only unique per user. With a global key, one user's
-- id could make another user's event look like a duplicate.
ALTER TABLE seen_events DROP CONSTRAINT seen_events_pkey;
ALTER TABLE seen_events ADD PRIMARY KEY (user_id, client_event_id);
