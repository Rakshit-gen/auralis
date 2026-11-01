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
}
