package com.auralis.notification.web;

import com.auralis.notification.store.Notification;
import com.auralis.notification.store.NotificationStore;
import java.time.OffsetDateTime;
import java.time.format.DateTimeParseException;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestAttribute;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

/**
 * The caller's inbox, mounted by the gateway at /api/notifications. Every
 * route is scoped to the identity the IdentityFilter verified, so a user only
 * ever sees or changes their own rows.
 */
@RestController
@RequestMapping("/notifications")
public class NotificationController {

    private final NotificationStore store;

    public NotificationController(NotificationStore store) {
        this.store = store;
    }

    @GetMapping
    Map<String, Object> inbox(@RequestAttribute(Identity.ATTRIBUTE) Identity caller,
            @RequestParam(required = false) String limit,
            @RequestParam(required = false) String before) {
        int n = limit(limit);
        OffsetDateTime beforeAt = null;
        UUID beforeId = null;
        if (before != null && !before.isEmpty()) {
            int i = before.lastIndexOf('_');
            try {
                beforeAt = OffsetDateTime.parse(before.substring(0, Math.max(i, 0)));
                beforeId = UUID.fromString(before.substring(i + 1));
            } catch (DateTimeParseException | IllegalArgumentException e) {
                throw ApiException.badRequest("invalid before cursor");
            }
        }
        List<Notification> page = store.inbox(caller.userId(), n, beforeAt, beforeId);
        Map<String, Object> out = new HashMap<>();
        out.put("notifications", page);
        out.put("unread", store.unreadCount(caller.userId()));
        out.put("next_before", page.size() < n ? null : cursor(page.getLast()));
        return out;
    }

    @GetMapping("/unread-count")
    Map<String, Long> unreadCount(@RequestAttribute(Identity.ATTRIBUTE) Identity caller) {
        return Map.of("unread", store.unreadCount(caller.userId()));
    }

    @PostMapping("/{id}/read")
    Map<String, Boolean> markRead(@RequestAttribute(Identity.ATTRIBUTE) Identity caller, @PathVariable UUID id) {
        if (!store.markRead(caller.userId(), id)) {
            throw ApiException.notFound("notification not found");
        }
        return Map.of("read", true);
    }

    @PostMapping("/read-all")
    Map<String, Integer> markAllRead(@RequestAttribute(Identity.ATTRIBUTE) Identity caller) {
        return Map.of("updated", store.markAllRead(caller.userId()));
    }

    /** Default 20, clamped to 1..100. A non-number falls back to the default, as page() does in Go. */
    static int limit(String raw) {
        try {
            return Math.min(Math.max(Integer.parseInt(raw), 1), 100);
        } catch (NumberFormatException e) {
            return 20;
        }
    }

    static String cursor(Notification n) {
        return n.created_at() + "_" + n.id();
    }
}
