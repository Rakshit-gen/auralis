package com.auralis.notification.store;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

/**
 * Deletes processed_events rows older than 14 days, the same window the
 * other services keep. The table records every consumed event id and would
 * otherwise grow with all traffic. Several instances running it at once is
 * harmless.
 */
@Component
public class Pruner {

    private static final Logger log = LoggerFactory.getLogger(Pruner.class);

    private final JdbcTemplate db;

    public Pruner(JdbcTemplate db) {
        this.db = db;
    }

    @Scheduled(initialDelayString = "PT1M", fixedDelayString = "PT1H")
    public int pruneProcessedEvents() {
        try {
            int n = db.update("DELETE FROM processed_events WHERE handled_at < now() - interval '14 days'");
            if (n > 0) {
                log.info("pruned old rows table=processed_events rows={}", n);
            }
            return n;
        } catch (RuntimeException e) {
            log.warn("prune failed table=processed_events", e);
            return 0;
        }
    }
}
