package com.auralis.notification.web;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.time.Instant;
import java.util.List;
import java.util.UUID;
import org.junit.jupiter.api.Test;

class IdentityTest {

    private static final String SECRET = "test-identity-secret";
    private static final String USER = "7a1f4c2e-9b3d-4e5f-8a6b-1c2d3e4f5a6b";

    private static String now() {
        return Long.toString(Instant.now().getEpochSecond());
    }

    @Test
    void matchesTheGoSignature() {
        // Produced by authn.IdentityHeaders in libs/go-platform with the same
        // secret and inputs, so a drift in either implementation fails here.
        assertThat(Identity.sign(SECRET, USER + "\nUSER,CREATOR\nreq-1\n1767225600"))
                .isEqualTo("f711cf87fb58463af6c3e4a6b8a3270b4fe5633cfee2a16c7c8e6b40c1e2ff1e");
    }

    @Test
    void acceptsAValidSignature() {
        String at = now();
        String sig = Identity.sign(SECRET, USER + "\nUSER,CREATOR\nreq-1\n" + at);
        Identity id = Identity.verify(SECRET, USER, "USER,CREATOR", "req-1", at, sig);
        assertThat(id.userId()).isEqualTo(UUID.fromString(USER));
        assertThat(id.roles()).isEqualTo(List.of("USER", "CREATOR"));
    }

    @Test
    void rejectsATamperedRole() {
        String at = now();
        String sig = Identity.sign(SECRET, USER + "\nUSER\nreq-1\n" + at);
        assertThatThrownBy(() -> Identity.verify(SECRET, USER, "USER,ADMIN", "req-1", at, sig))
                .hasMessageContaining("signature mismatch");
    }

    @Test
    void rejectsAnOldSignature() {
        String at = Long.toString(Instant.now().getEpochSecond() - 600);
        String sig = Identity.sign(SECRET, USER + "\nUSER\nreq-1\n" + at);
        assertThatThrownBy(() -> Identity.verify(SECRET, USER, "USER", "req-1", at, sig))
                .hasMessageContaining("expired");
    }

    @Test
    void rejectsMissingHeaders() {
        assertThatThrownBy(() -> Identity.verify(SECRET, null, null, null, null, null))
                .hasMessageContaining("missing identity headers");
    }
}
