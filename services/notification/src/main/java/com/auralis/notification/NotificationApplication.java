// Package com.auralis.notification is the Auralis notification service. It
// keeps a projection of who follows which show and, when a new episode is
// published, writes an in-app notification for every follower.
package com.auralis.notification;

import com.auralis.notification.config.Env;
import java.util.HashMap;
import java.util.Map;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.WebApplicationType;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class NotificationApplication {

    public static void main(String[] args) {
        SpringApplication app = new SpringApplication(NotificationApplication.class);
        // Defaults have the lowest precedence, so an explicit spring.* setting
        // still wins over the translated platform variables.
        Map<String, Object> defaults = new HashMap<>();
        defaults.putAll(Env.httpAddr(System.getenv().getOrDefault("NOTIFICATION_HTTP_ADDR", ":8088")));
        defaults.putAll(Env.databaseUrl(System.getenv("NOTIFICATION_DATABASE_URL")));
        defaults.putAll(Env.kafkaSecurity(System.getenv()));

        // "notification migrate" applies Flyway migrations and exits, like the
        // Go services' "migrate" argument that `make migrate` relies on.
        boolean migrateOnly = args.length > 0 && args[0].equals("migrate");
        if (migrateOnly) {
            app.setWebApplicationType(WebApplicationType.NONE);
            defaults.put("NOTIFICATION_RUN_CONSUMER", "false");
        }
        app.setDefaultProperties(defaults);
        var ctx = app.run(args);
        if (migrateOnly) {
            System.exit(SpringApplication.exit(ctx));
        }
    }
}
