package com.auralis.notification.store;

import java.util.UUID;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Repository;

/** The show_followers projection. */
@Repository
public class FollowerStore {

    private final JdbcTemplate db;

    public FollowerStore(JdbcTemplate db) {
        this.db = db;
    }

    public void follow(UUID showId, UUID userId) {
        db.update("INSERT INTO show_followers (show_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
                showId, userId);
    }

    public void unfollow(UUID showId, UUID userId) {
        db.update("DELETE FROM show_followers WHERE show_id = ? AND user_id = ?", showId, userId);
    }
}
