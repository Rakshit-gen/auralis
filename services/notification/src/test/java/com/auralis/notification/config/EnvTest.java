package com.auralis.notification.config;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.Map;
import org.junit.jupiter.api.Test;

class EnvTest {

    @Test
    void portOnlyAddr() {
        assertThat(Env.httpAddr(":8088")).isEqualTo(Map.of("server.port", "8088"));
    }

    @Test
    void hostAndPortAddr() {
        assertThat(Env.httpAddr("0.0.0.0:10000"))
                .isEqualTo(Map.of("server.port", "10000", "server.address", "0.0.0.0"));
    }

    @Test
    void badAddrFailsFast() {
        assertThatThrownBy(() -> Env.httpAddr("8088")).isInstanceOf(IllegalArgumentException.class);
    }

    @Test
    void postgresUrlBecomesJdbc() {
        Map<String, String> p = Env.databaseUrl(
                "postgres://auralis:p%40ss@postgres:5432/notification?sslmode=disable");
        assertThat(p).containsEntry("spring.datasource.url",
                        "jdbc:postgresql://postgres:5432/notification?sslmode=disable")
                .containsEntry("spring.datasource.username", "auralis")
                .containsEntry("spring.datasource.password", "p@ss");
    }

    @Test
    void defaultPortIsFilledIn() {
        assertThat(Env.databaseUrl("postgresql://u:p@ep-x.neon.tech/db?sslmode=require"))
                .containsEntry("spring.datasource.url", "jdbc:postgresql://ep-x.neon.tech:5432/db?sslmode=require");
    }

    @Test
    void jdbcUrlPassesThrough() {
        assertThat(Env.databaseUrl("jdbc:postgresql://localhost/x"))
                .isEqualTo(Map.of("spring.datasource.url", "jdbc:postgresql://localhost/x"));
    }

    @Test
    void noKafkaSecurityMeansPlaintext() {
        assertThat(Env.kafkaSecurity(Map.of())).isEmpty();
    }

    @Test
    void scramImpliesSaslSsl() {
        Map<String, String> p = Env.kafkaSecurity(Map.of(
                "KAFKA_SASL_MECHANISM", "scram-sha-256",
                "KAFKA_SASL_USERNAME", "svc",
                "KAFKA_SASL_PASSWORD", "pa\"ss"));
        assertThat(p).containsEntry("spring.kafka.properties.security.protocol", "SASL_SSL")
                .containsEntry("spring.kafka.properties.sasl.mechanism", "SCRAM-SHA-256")
                .containsEntry("spring.kafka.properties.sasl.jaas.config",
                        "org.apache.kafka.common.security.scram.ScramLoginModule required "
                                + "username=\"svc\" password=\"pa\\\"ss\";");
    }

    @Test
    void tlsWithoutSasl() {
        assertThat(Env.kafkaSecurity(Map.of("KAFKA_TLS_ENABLED", "true")))
                .isEqualTo(Map.of("spring.kafka.properties.security.protocol", "SSL"));
    }

    @Test
    void mechanismWithoutCredentialsFailsFast() {
        assertThatThrownBy(() -> Env.kafkaSecurity(Map.of("KAFKA_SASL_MECHANISM", "PLAIN")))
                .hasMessageContaining("USERNAME/PASSWORD");
    }
}
