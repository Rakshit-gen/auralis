package com.auralis.notification;

import com.auralis.notification.config.Env;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;

/**
 * Base for tests that need Postgres. Like the Go services, they run only when
 * NOTIFICATION_TEST_DATABASE_URL points at a disposable database.
 */
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT, properties = {"IDENTITY_SECRET=test-identity-secret", "NOTIFICATION_RUN_CONSUMER=false"})
@EnabledIfEnvironmentVariable(named = "NOTIFICATION_TEST_DATABASE_URL", matches = ".+")
public abstract class DatabaseTest {

    @Autowired
    protected JdbcTemplate db;

    @DynamicPropertySource
    static void database(DynamicPropertyRegistry r) {
        Env.databaseUrl(System.getenv("NOTIFICATION_TEST_DATABASE_URL")).forEach((k, v) -> r.add(k, () -> v));
    }

    @BeforeEach
    void truncate() {
        db.execute("TRUNCATE processed_events, show_followers, notifications");
    }
}
