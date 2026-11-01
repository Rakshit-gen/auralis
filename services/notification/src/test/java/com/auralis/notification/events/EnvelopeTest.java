package com.auralis.notification.events;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import org.junit.jupiter.api.Test;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

class EnvelopeTest {

    private final ObjectMapper json = JsonMapper.builder().build();

    private static final String VALID = """
            {"event_id":"b3b1c0de-0000-4000-8000-000000000001","event_type":"user.followed",
             "event_version":1,"occurred_at":"2026-09-08T12:34:56.000Z","producer":"user",
             "correlation_id":"c1","payload":{"user_id":"u1","show_id":"s1"}}
            """;

    @Test
    void parsesAValidEnvelope() {
        Envelope e = Envelope.parse(json, VALID);
        assertThat(e.eventType()).isEqualTo("user.followed");
        assertThat(e.eventVersion()).isEqualTo(1);
        assertThat(e.correlationId()).isEqualTo("c1");
        assertThat(e.field("show_id")).isEqualTo("s1");
        assertThat(e.field("missing")).isEmpty();
    }

    @Test
    void rejectsGarbage() {
        assertThatThrownBy(() -> Envelope.parse(json, "not json"))
                .isInstanceOf(InvalidEnvelopeException.class);
        assertThatThrownBy(() -> Envelope.parse(json, "[1,2]"))
                .isInstanceOf(InvalidEnvelopeException.class);
    }

    @Test
    void rejectsNonUuidEventId() {
        assertThatThrownBy(() -> Envelope.parse(json, VALID.replace("b3b1c0de-0000-4000-8000-000000000001", "abc")))
                .hasMessageContaining("must be a UUID");
    }

    @Test
    void rejectsVersionZero() {
        assertThatThrownBy(() -> Envelope.parse(json, VALID.replace("\"event_version\":1", "\"event_version\":0")))
                .hasMessageContaining("event_version");
    }

    @Test
    void rejectsMissingPayload() {
        assertThatThrownBy(() -> Envelope.parse(json, VALID.replace("\"payload\"", "\"other\"")))
                .hasMessageContaining("payload is required");
    }
}
