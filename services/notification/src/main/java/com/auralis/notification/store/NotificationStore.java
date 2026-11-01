package com.auralis.notification.store;

import java.time.OffsetDateTime;
import java.util.List;
import java.util.UUID;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.stereotype.Repository;

/** Writes and reads the notifications table. */
@Repository
public class NotificationStore {

    public static final String KIND_NEW_EPISODE = "new_episode";

    private static final RowMapper<Notification> ROW = (rs, i) -> new Notification(
            rs.getObject("id", UUID.class),
            rs.getString("kind"),
            rs.getObject("show_id", UUID.class),
            rs.getObject("episode_id", UUID.class),
            rs.getString("title"),
            rs.getString("body"),
            rs.getObject("created_at", OffsetDateTime.class),
            rs.getObject("read_at", OffsetDateTime.class));

    private final JdbcTemplate db;

    public NotificationStore(JdbcTemplate db) {
        this.db = db;
    }

    /**
     * Notifies every follower of showId about a new episode in one statement,
     * so fan-out cost is one round trip however many followers a show has.
     * The show's creator is skipped. Returns the number of rows written; a
     * follower already notified about this episode is left alone.
     */
    public int fanOutNewEpisode(UUID showId, UUID episodeId, UUID creatorId, String title, String body) {
        return db.update("""
                INSERT INTO notifications (user_id, kind, show_id, episode_id, title, body)
                SELECT f.user_id, ?, f.show_id, ?, ?, ?
                FROM show_followers f
                WHERE f.show_id = ? AND f.user_id IS DISTINCT FROM ?
                ON CONFLICT (user_id, kind, episode_id) DO NOTHING
                """, KIND_NEW_EPISODE, episodeId, title, body, showId, creatorId);
    }
}
