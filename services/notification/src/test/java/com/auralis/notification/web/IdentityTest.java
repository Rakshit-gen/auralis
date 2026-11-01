package com.auralis.notification.web;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.List;
import java.util.UUID;
import org.junit.jupiter.api.Test;

class IdentityTest {

    private static final String SECRET = "test-identity-secret";
    private static final String USER = "7a1f4c2e-9b3d-4e5f-8a6b-1c2d3e4f5a6b";

    @Test
    void matchesTheGoSignature() {
        // Produced by authn.IdentityHeaders in libs/go-platform with the same
        // secret and inputs, so a drift in either implementation fails here.
        assertThat(Identity.sign(SECRET, USER + "\nUSER,CREATOR\nreq-1"))
                .isEqualTo("b2659501777b6c5da7dc3d7745a03d3dd0ec93d2a92cb74b06dfd88a55055eb1");
    }

    @Test
    void acceptsAValidSignature() {
        String sig = Identity.sign(SECRET, USER + "\nUSER,CREATOR\nreq-1");
        Identity id = Identity.verify(SECRET, USER, "USER,CREATOR", "req-1", sig);
        assertThat(id.userId()).isEqualTo(UUID.fromString(USER));
        assertThat(id.roles()).isEqualTo(List.of("USER", "CREATOR"));
    }

    @Test
    void rejectsATamperedRole() {
        String sig = Identity.sign(SECRET, USER + "\nUSER\nreq-1");
        assertThatThrownBy(() -> Identity.verify(SECRET, USER, "USER,ADMIN", "req-1", sig))
                .hasMessageContaining("signature mismatch");
    }

    @Test
    void rejectsMissingHeaders() {
        assertThatThrownBy(() -> Identity.verify(SECRET, null, null, null, null))
                .hasMessageContaining("missing identity headers");
    }
}
