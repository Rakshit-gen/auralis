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

    /**
     * Kafka transport security from the same variables kafkax/security.go
     * reads: KAFKA_SASL_MECHANISM (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512),
     * KAFKA_SASL_USERNAME / _PASSWORD, KAFKA_TLS_ENABLED and
     * KAFKA_TLS_SKIP_VERIFY. Empty mechanism and no TLS means PLAINTEXT.
     * Skip-verify only turns off hostname checks here; the Java client has no
     * one-flag equivalent of Go's InsecureSkipVerify.
     */
    public static Map<String, String> kafkaSecurity(Map<String, String> env) {
        Map<String, String> out = new HashMap<>();
        String mech = env.getOrDefault("KAFKA_SASL_MECHANISM", "").trim().toUpperCase();
        boolean tls = !mech.isEmpty() || truthy(env.get("KAFKA_TLS_ENABLED"));
        if (!mech.isEmpty()) {
            String user = env.getOrDefault("KAFKA_SASL_USERNAME", "");
            String pass = env.getOrDefault("KAFKA_SASL_PASSWORD", "");
            if (user.isEmpty() || pass.isEmpty()) {
                throw new IllegalArgumentException(
                        "KAFKA_SASL_MECHANISM is set but KAFKA_SASL_USERNAME/PASSWORD are empty");
            }
            String module = switch (mech) {
                case "PLAIN" -> "org.apache.kafka.common.security.plain.PlainLoginModule";
                case "SCRAM-SHA-256", "SCRAM-SHA-512" -> "org.apache.kafka.common.security.scram.ScramLoginModule";
                default -> throw new IllegalArgumentException("unsupported KAFKA_SASL_MECHANISM " + mech);
            };
            out.put("spring.kafka.properties.sasl.mechanism", mech);
            out.put("spring.kafka.properties.sasl.jaas.config", module + " required username=\"" + jaas(user)
                    + "\" password=\"" + jaas(pass) + "\";");
        }
        if (tls) {
            out.put("spring.kafka.properties.security.protocol", mech.isEmpty() ? "SSL" : "SASL_SSL");
            if (truthy(env.get("KAFKA_TLS_SKIP_VERIFY"))) {
                out.put("spring.kafka.properties.ssl.endpoint.identification.algorithm", "");
            }
        }
        return out;
    }

    private static boolean truthy(String v) {
        return v != null && switch (v.trim().toLowerCase()) {
            case "1", "true", "yes", "on" -> true;
            default -> false;
        };
    }

    private static String jaas(String s) {
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }
}
