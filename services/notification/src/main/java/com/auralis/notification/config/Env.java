package com.auralis.notification.config;

import java.net.URI;
import java.net.URLDecoder;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;

/**
 * Translates the platform's environment conventions into Spring properties.
 * Every Auralis service is configured the same way (an HTTP_ADDR like ":8088"
 * and a libpq-style postgres:// URL), so the Java service reads the same
 * variables instead of growing its own.
 */
public final class Env {

    private Env() {}

    /** Splits "host:port" or ":port" into server.address / server.port. */
    public static Map<String, String> httpAddr(String addr) {
        Map<String, String> out = new HashMap<>();
        if (addr == null || addr.isBlank()) {
            return out;
        }
        int i = addr.lastIndexOf(':');
        if (i < 0) {
            throw new IllegalArgumentException("HTTP addr must look like host:port or :port, got " + addr);
        }
        String host = addr.substring(0, i);
        out.put("server.port", String.valueOf(Integer.parseInt(addr.substring(i + 1))));
        if (!host.isEmpty()) {
            out.put("server.address", host);
        }
        return out;
    }

    /**
     * Converts postgres://user:pass@host:port/db?sslmode=x into a JDBC URL plus
     * separate credentials. Query parameters (sslmode, channel_binding) pass
     * through, since the Postgres JDBC driver understands the same names.
     */
    public static Map<String, String> databaseUrl(String url) {
        Map<String, String> out = new HashMap<>();
        if (url == null || url.isBlank()) {
            return out;
        }
        if (url.startsWith("jdbc:")) {
            out.put("spring.datasource.url", url);
            return out;
        }
        URI u = URI.create(url);
        if (!"postgres".equals(u.getScheme()) && !"postgresql".equals(u.getScheme())) {
            throw new IllegalArgumentException("database URL must be postgres://, got scheme " + u.getScheme());
        }
        int port = u.getPort() == -1 ? 5432 : u.getPort();
        String jdbc = "jdbc:postgresql://" + u.getHost() + ":" + port + u.getRawPath();
        if (u.getRawQuery() != null) {
            jdbc += "?" + u.getRawQuery();
        }
        out.put("spring.datasource.url", jdbc);
        if (u.getRawUserInfo() != null) {
            String[] parts = u.getRawUserInfo().split(":", 2);
            out.put("spring.datasource.username", decode(parts[0]));
            if (parts.length == 2) {
                out.put("spring.datasource.password", decode(parts[1]));
            }
        }
        return out;
    }

    private static String decode(String s) {
        return URLDecoder.decode(s, StandardCharsets.UTF_8);
    }
}
