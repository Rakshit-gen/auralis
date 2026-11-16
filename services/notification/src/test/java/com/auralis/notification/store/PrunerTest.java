package com.auralis.notification.store;

import static org.junit.jupiter.api.Assertions.assertEquals;

import com.auralis.notification.DatabaseTest;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;

class PrunerTest extends DatabaseTest {

    @Autowired
    Pruner pruner;

    @Test
    void dropsOnlyOldProcessedEvents() {
        db.update("""
                INSERT INTO processed_events (consumer, event_id, handled_at) VALUES
                ('c', gen_random_uuid(), now() - interval '15 days'),
                ('c', gen_random_uuid(), now())""");
        assertEquals(1, pruner.pruneProcessedEvents());
        assertEquals(1, db.queryForObject("SELECT count(*) FROM processed_events", Integer.class));
    }
}
