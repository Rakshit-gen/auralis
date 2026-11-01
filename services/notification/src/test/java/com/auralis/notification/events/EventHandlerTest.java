package com.auralis.notification.events;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.auralis.notification.DatabaseTest;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import tools.jackson.databind.ObjectMapper;

class EventHandlerTest extends DatabaseTest {

    @Autowired
    EventHandler handler;

    @Autowired
    ObjectMapper json;

    final UUID show = UUID.randomUUID();
    final UUID creator = UUID.randomUUID();
    final UUID fan = UUID.randomUUID();

    Envelope event(UUID id, String type, String payload) {
        return event(id, type, payload, Instant.now());
    }

    Envelope event(UUID id, String type, String payload, Instant at) {
        return Envelope.parse(json, """
                {"event_id":"%s","event_type":"%s","event_version":1,
                 "occurred_at":"%s","producer":"test","payload":%s}
                """.formatted(id, type, at, payload));
    }

    Envelope follow(UUID user) {
        return event(UUID.randomUUID(), "user.followed",
                "{\"user_id\":\"%s\",\"show_id\":\"%s\"}".formatted(user, show));
    }

    Envelope published(UUID eventId, UUID episode) {
        return event(eventId, "content.episode_published", """
                {"show_id":"%s","title":"The Vantage","creator_id":"%s","episode_id":"%s",
                 "episode_number":3,"episode_title":"Signal Loss"}
                """.formatted(show, creator, episode));
    }

    long notifications(UUID user) {
        return db.queryForObject("SELECT count(*) FROM notifications WHERE user_id = ?", Long.class, user);
    }

    @Test
    void followersAreNotifiedButNotTheCreator() {
        handler.handle(follow(fan));
        handler.handle(follow(creator));
        handler.handle(published(UUID.randomUUID(), UUID.randomUUID()));

        assertThat(notifications(fan)).isEqualTo(1);
        assertThat(notifications(creator)).isZero();
        assertThat(db.queryForObject("SELECT title || ' / ' || body FROM notifications", String.class))
                .isEqualTo("New episode of The Vantage / Episode 3: Signal Loss");
    }

    @Test
    void redeliveredEventIsANoOp() {
        handler.handle(follow(fan));
        Envelope e = published(UUID.randomUUID(), UUID.randomUUID());
        handler.handle(e);
        handler.handle(e);
        assertThat(notifications(fan)).isEqualTo(1);
    }

    @Test
    void republishingAnEpisodeDoesNotNotifyTwice() {
        handler.handle(follow(fan));
        UUID episode = UUID.randomUUID();
        handler.handle(published(UUID.randomUUID(), episode));
        handler.handle(published(UUID.randomUUID(), episode));
        assertThat(notifications(fan)).isEqualTo(1);
    }

    @Test
    void unfollowStopsFutureNotifications() {
        handler.handle(follow(fan));
        handler.handle(event(UUID.randomUUID(), "user.unfollowed",
                "{\"user_id\":\"%s\",\"show_id\":\"%s\"}".formatted(fan, show)));
        handler.handle(published(UUID.randomUUID(), UUID.randomUUID()));
        assertThat(notifications(fan)).isZero();
    }

    @Test
    void badPayloadRollsBackTheLedger() {
        UUID id = UUID.randomUUID();
        assertThatThrownBy(() -> handler.handle(event(id, "user.followed", "{\"user_id\":\"nope\"}")))
                .isInstanceOf(InvalidEnvelopeException.class);
        assertThat(db.queryForObject("SELECT count(*) FROM processed_events WHERE event_id = ?", Long.class, id))
                .isZero();
    }

    @Test
    void unknownVersionIsSkipped() {
        handler.handle(follow(fan));
        handler.handle(Envelope.parse(json, """
                {"event_id":"%s","event_type":"content.episode_published","event_version":2,
                 "occurred_at":"2026-09-08T12:00:00Z","producer":"test","payload":{}}
                """.formatted(UUID.randomUUID())));
        assertThat(notifications(fan)).isZero();
    }

    @Test
    void oldPublishFromAReplayDoesNotNotify() {
        handler.handle(follow(fan));
        handler.handle(event(UUID.randomUUID(), "content.episode_published", """
                {"show_id":"%s","title":"The Vantage","creator_id":"%s","episode_id":"%s",
                 "episode_number":1,"episode_title":"Pilot"}
                """.formatted(show, creator, UUID.randomUUID()), Instant.now().minus(3, ChronoUnit.DAYS)));
        assertThat(notifications(fan)).isZero();
        assertThat(db.queryForObject("SELECT count(*) FROM show_followers", Long.class)).isEqualTo(1);
    }
}
