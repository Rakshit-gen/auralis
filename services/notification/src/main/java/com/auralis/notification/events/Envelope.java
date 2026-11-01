package com.auralis.notification.events;

import java.time.OffsetDateTime;
import java.util.UUID;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/**
 * The shared Kafka envelope. Field rules match libs/go-platform/envelope, so an
 * event the Go consumers reject is rejected here too.
 */
public record Envelope(
        UUID eventId,
        String eventType,
        int eventVersion,
        OffsetDateTime occurredAt,
        String producer,
        String correlationId,
        JsonNode payload) {

    /** Parses and validates one message value. Throws InvalidEnvelopeException on any problem. */
    public static Envelope parse(ObjectMapper json, String raw) {
        JsonNode n;
        try {
            n = json.readTree(raw);
        } catch (JacksonException e) {
            throw new InvalidEnvelopeException("envelope: not valid JSON");
        }
        if (n == null || !n.isObject()) {
            throw new InvalidEnvelopeException("envelope: not a JSON object");
        }
        String id = text(n, "event_id");
        String type = text(n, "event_type");
        int version = n.path("event_version").asInt(0);
        String occurred = text(n, "occurred_at");
        String producer = text(n, "producer");
        JsonNode payload = n.get("payload");

        if (id.isEmpty()) throw new InvalidEnvelopeException("envelope: event_id is required");
        if (type.isEmpty()) throw new InvalidEnvelopeException("envelope: event_type is required");
        if (version < 1) throw new InvalidEnvelopeException("envelope: event_version must be >= 1");
        if (occurred.isEmpty()) throw new InvalidEnvelopeException("envelope: occurred_at is required");
        if (producer.isEmpty()) throw new InvalidEnvelopeException("envelope: producer is required");
        if (payload == null || payload.isNull()) throw new InvalidEnvelopeException("envelope: payload is required");

        UUID eventId;
        try {
            eventId = UUID.fromString(id);
        } catch (IllegalArgumentException e) {
            throw new InvalidEnvelopeException("envelope: event_id must be a UUID");
        }
        OffsetDateTime at;
        try {
            at = OffsetDateTime.parse(occurred);
        } catch (RuntimeException e) {
            throw new InvalidEnvelopeException("envelope: occurred_at must be RFC 3339");
        }
        return new Envelope(eventId, type, version, at, producer, text(n, "correlation_id"), payload);
    }

    /** Reads a string payload field, or "" when absent. */
    public String field(String name) {
        return text(payload, name);
    }

    private static String text(JsonNode n, String name) {
        JsonNode v = n.get(name);
        return v == null || v.isNull() ? "" : v.asString();
    }
}
