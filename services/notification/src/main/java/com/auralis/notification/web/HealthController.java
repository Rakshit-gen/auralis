package com.auralis.notification.web;

import java.util.Map;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

/** /health and /ready with the same bodies as libs/go-platform/health. */
@RestController
public class HealthController {

    static final String VERSION = "1.0.0";

    private final JdbcTemplate db;

    public HealthController(JdbcTemplate db) {
        this.db = db;
    }

    @GetMapping("/health")
    Map<String, String> health() {
        return Map.of("status", "ok", "service", "notification", "version", VERSION);
    }

    @GetMapping("/ready")
    ResponseEntity<Map<String, Object>> ready() {
        String database;
        try {
            db.queryForObject("SELECT 1", Integer.class);
            database = "ok";
        } catch (RuntimeException e) {
            database = e.getMessage();
        }
        boolean ok = database.equals("ok");
        return ResponseEntity.status(ok ? HttpStatus.OK : HttpStatus.SERVICE_UNAVAILABLE)
                .body(Map.of("status", ok ? "ready" : "not_ready", "checks", Map.of("database", database)));
    }
}
