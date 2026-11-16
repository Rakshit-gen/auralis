package com.auralis.notification.web;

import static org.assertj.core.api.Assertions.assertThatCode;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import org.junit.jupiter.api.Test;

class IdentityFilterTest {

    private static final String DEV = "dev-identity-secret-change-me-0123456789";

    @Test
    void refusesShortSecrets() {
        assertThatThrownBy(() -> IdentityFilter.checkSecret("short", true)).isInstanceOf(IllegalStateException.class);
    }

    @Test
    void refusesThePlaceholderUnlessAllowed() {
        assertThatThrownBy(() -> IdentityFilter.checkSecret(DEV, false)).isInstanceOf(IllegalStateException.class);
        assertThatCode(() -> IdentityFilter.checkSecret(DEV, true)).doesNotThrowAnyException();
    }

    @Test
    void acceptsARealSecret() {
        assertThatCode(() -> IdentityFilter.checkSecret("9f8e7d6c5b4a39281706f5e4d3c2b1a0aa", false))
                .doesNotThrowAnyException();
    }
}
