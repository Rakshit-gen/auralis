package com.auralis.notification.web;

import static org.assertj.core.api.Assertions.assertThat;

import com.auralis.notification.DatabaseTest;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

class NotificationApiTest extends DatabaseTest {

    @Value("${local.server.port}")
    int port;

    @Autowired
    ObjectMapper json;

    final HttpClient http = HttpClient.newHttpClient();
    final UUID me = UUID.randomUUID();
    final UUID other = UUID.randomUUID();

    record Res(int status, JsonNode body) {}

    Res call(String method, String path, UUID user) throws Exception {
        HttpRequest.Builder b = HttpRequest.newBuilder(URI.create("http://localhost:" + port + path))
                .method(method, HttpRequest.BodyPublishers.noBody());
        if (user != null) {
            b.header("X-Auralis-User", user.toString())
                    .header("X-Auralis-Roles", "USER")
                    .header("X-Auralis-Request-Id", "req-1")
                    .header("X-Auralis-Identity-Sig",
                            Identity.sign("test-identity-secret", user + "\nUSER\nreq-1"));
        }
        HttpResponse<String> r = http.send(b.build(), HttpResponse.BodyHandlers.ofString());
        return new Res(r.statusCode(), json.readTree(r.body()));
    }

    UUID insert(UUID user, String createdAt) {
        return db.queryForObject("""
                INSERT INTO notifications (user_id, kind, show_id, episode_id, title, created_at)
                VALUES (?, 'new_episode', gen_random_uuid(), gen_random_uuid(), 't', ?::timestamptz)
                RETURNING id
                """, UUID.class, user, createdAt);
    }

    @Test
    void missingIdentityIs401WithThePlatformEnvelope() throws Exception {
        Res r = call("GET", "/notifications", null);
        assertThat(r.status()).isEqualTo(401);
        assertThat(r.body().at("/error/code").asString()).isEqualTo("auth.unauthorized");
    }

    @Test
    void forgedSignatureIs401() throws Exception {
        HttpRequest req = HttpRequest.newBuilder(URI.create("http://localhost:" + port + "/notifications"))
                .header("X-Auralis-User", me.toString())
                .header("X-Auralis-Roles", "ADMIN")
                .header("X-Auralis-Identity-Sig", "00").build();
        assertThat(http.send(req, HttpResponse.BodyHandlers.ofString()).statusCode()).isEqualTo(401);
    }

    @Test
    void healthNeedsNoIdentity() throws Exception {
        assertThat(call("GET", "/health", null).body().get("service").asString()).isEqualTo("notification");
    }

    @Test
    void inboxPagesNewestFirstAndOnlyShowsMine() throws Exception {
        UUID oldest = insert(me, "2026-10-01T10:00:00Z");
        UUID middle = insert(me, "2026-10-02T10:00:00Z");
        UUID newest = insert(me, "2026-10-03T10:00:00Z");
        insert(other, "2026-10-04T10:00:00Z");

        Res first = call("GET", "/notifications?limit=2", me);
        assertThat(first.body().get("unread").asLong()).isEqualTo(3);
        assertThat(first.body().get("notifications").get(0).get("id").asString()).isEqualTo(newest.toString());
        assertThat(first.body().get("notifications").get(1).get("id").asString()).isEqualTo(middle.toString());

        String cursor = first.body().get("next_before").asString();
        Res second = call("GET", "/notifications?limit=2&before=" + cursor, me);
        assertThat(second.body().get("notifications")).hasSize(1);
        assertThat(second.body().get("notifications").get(0).get("id").asString()).isEqualTo(oldest.toString());
        assertThat(second.body().get("next_before").isNull()).isTrue();
    }

    @Test
    void badCursorIs400() throws Exception {
        assertThat(call("GET", "/notifications?before=nope", me).status()).isEqualTo(400);
    }

    @Test
    void cannotMarkSomeoneElsesNotification() throws Exception {
        UUID theirs = insert(other, "2026-10-01T10:00:00Z");
        assertThat(call("POST", "/notifications/" + theirs + "/read", me).status()).isEqualTo(404);
        assertThat(call("GET", "/notifications/unread-count", other).body().get("unread").asLong()).isEqualTo(1);
    }

    @Test
    void markReadAndReadAll() throws Exception {
        UUID a = insert(me, "2026-10-01T10:00:00Z");
        insert(me, "2026-10-02T10:00:00Z");
        insert(me, "2026-10-03T10:00:00Z");

        assertThat(call("POST", "/notifications/" + a + "/read", me).status()).isEqualTo(200);
        assertThat(call("GET", "/notifications/unread-count", me).body().get("unread").asLong()).isEqualTo(2);
        assertThat(call("POST", "/notifications/read-all", me).body().get("updated").asInt()).isEqualTo(2);
        assertThat(call("GET", "/notifications/unread-count", me).body().get("unread").asLong()).isZero();
    }

    @Test
    void nonUuidIdIs400() throws Exception {
        Res r = call("POST", "/notifications/abc/read", me);
        assertThat(r.status()).isEqualTo(400);
        assertThat(r.body().at("/error/code").asString()).isEqualTo("request.validation_failed");
    }
}
