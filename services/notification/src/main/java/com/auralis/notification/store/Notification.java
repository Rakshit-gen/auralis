package com.auralis.notification.store;

import java.time.OffsetDateTime;
import java.util.UUID;

/** One inbox entry, serialized with the snake_case field names the API uses. */
public record Notification(
        UUID id,
        String kind,
        UUID show_id,
        UUID episode_id,
        String title,
        String body,
        OffsetDateTime created_at,
        OffsetDateTime read_at) {}
