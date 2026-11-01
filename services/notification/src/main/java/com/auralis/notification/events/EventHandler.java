package com.auralis.notification.events;

import com.auralis.notification.store.FollowerStore;
import com.auralis.notification.store.NotificationStore;
import io.micrometer.core.instrument.MeterRegistry;
import java.util.UUID;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Applies one domain event. The processed_events insert and every write it
 * drives commit together, so a redelivered event is a no-op.
 */
@Service
public class EventHandler {

    public static final String CONSUMER_GROUP = "notification-service";

    private static final Logger log = LoggerFactory.getLogger(EventHandler.class);

    private final JdbcTemplate db;
    private final FollowerStore followers;
    private final NotificationStore notifications;
    private final MeterRegistry metrics;

    public EventHandler(JdbcTemplate db, FollowerStore followers, NotificationStore notifications,
            MeterRegistry metrics) {
        this.db = db;
        this.followers = followers;
        this.notifications = notifications;
        this.metrics = metrics;
    }

    @Transactional
    public void handle(Envelope env) {
        int fresh = db.update(
                "INSERT INTO processed_events (consumer, event_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
                CONSUMER_GROUP, env.eventId());
        if (fresh == 0) {
            count("duplicate");
            return;
        }
        count(apply(env) ? "applied" : "ignored");
    }

    private boolean apply(Envelope env) {
        if (env.eventVersion() != 1) {
            // EVENT_CONTRACTS.md: an unknown version is a skip plus a metric, never a crash.
            log.warn("skipping unsupported event version {} v{}", env.eventType(), env.eventVersion());
            metrics.counter("notification_unsupported_version_total", "event_type", env.eventType()).increment();
            return false;
        }
        switch (env.eventType()) {
            case "user.followed" -> followers.follow(uuid(env, "show_id"), uuid(env, "user_id"));
            case "user.unfollowed" -> followers.unfollow(uuid(env, "show_id"), uuid(env, "user_id"));
            case "content.episode_published" -> episodePublished(env);
            default -> {
                return false;
            }
        }
        return true;
    }

    private void episodePublished(Envelope env) {
        UUID showId = uuid(env, "show_id");
        UUID episodeId = uuid(env, "episode_id");
        String creator = env.field("creator_id");
        String show = env.field("title");
        String episode = env.field("episode_title");
        int number = env.payload().path("episode_number").asInt(0);

        String title = "New episode of " + show;
        String body = number > 0 ? "Episode " + number + ": " + episode : episode;
        int sent = notifications.fanOutNewEpisode(showId, episodeId,
                creator.isEmpty() ? null : UUID.fromString(creator), title, body);
        metrics.counter("notification_sent_total", "kind", NotificationStore.KIND_NEW_EPISODE).increment(sent);
        log.info("episode {} of show {} notified {} followers", episodeId, showId, sent);
    }

    /** A required UUID payload field. A missing or malformed one can never succeed, so it is not retried. */
    private static UUID uuid(Envelope env, String name) {
        String v = env.field(name);
        try {
            return UUID.fromString(v);
        } catch (IllegalArgumentException e) {
            throw new InvalidEnvelopeException(env.eventType() + ": " + name + " must be a UUID, got '" + v + "'");
        }
    }

    private void count(String outcome) {
        metrics.counter("notification_consume_total", "outcome", outcome).increment();
    }
}
