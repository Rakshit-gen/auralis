package com.auralis.notification.web;

import java.nio.charset.StandardCharsets;
import java.security.InvalidKeyException;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.time.Instant;
import java.util.HexFormat;
import java.util.List;
import java.util.UUID;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

/**
 * The caller as asserted by the gateway. The gateway verifies the JWT and
 * forwards X-Auralis-User / -Roles / -Request-Id / -Issued-At plus an
 * HMAC-SHA256 over "user\nroles\nrequestId\nissuedAt" in
 * X-Auralis-Identity-Sig (see libs/go-platform/authn). Services trust the
 * headers only when the signature matches and they were signed in the last
 * minute.
 */
public record Identity(UUID userId, List<String> roles) {

    public static final String ATTRIBUTE = "auralis.identity";
    static final long MAX_AGE_SECONDS = 60;

    /** Verifies the signed headers. Throws ApiException(401) when they are missing or forged. */
    public static Identity verify(String secret, String user, String roles, String requestId, String issuedAt,
            String sig) {
        if (user == null || user.isEmpty() || sig == null || sig.isEmpty()) {
            throw ApiException.unauthorized("missing identity headers");
        }
        roles = roles == null ? "" : roles;
        requestId = requestId == null ? "" : requestId;
        issuedAt = issuedAt == null ? "" : issuedAt;
        byte[] want = sign(secret, user + "\n" + roles + "\n" + requestId + "\n" + issuedAt)
                .getBytes(StandardCharsets.UTF_8);
        if (!MessageDigest.isEqual(want, sig.getBytes(StandardCharsets.UTF_8))) {
            throw ApiException.unauthorized("identity signature mismatch");
        }
        try {
            long age = Instant.now().getEpochSecond() - Long.parseLong(issuedAt);
            if (Math.abs(age) > MAX_AGE_SECONDS) {
                throw ApiException.unauthorized("identity headers have expired");
            }
        } catch (NumberFormatException e) {
            throw ApiException.unauthorized("identity headers have expired");
        }
        UUID id;
        try {
            id = UUID.fromString(user);
        } catch (IllegalArgumentException e) {
            throw ApiException.unauthorized("identity user is not a UUID");
        }
        return new Identity(id, roles.isEmpty() ? List.of() : List.of(roles.split(",")));
    }

    static String sign(String secret, String message) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(secret.getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
            return HexFormat.of().formatHex(mac.doFinal(message.getBytes(StandardCharsets.UTF_8)));
        } catch (NoSuchAlgorithmException | InvalidKeyException e) {
            throw new IllegalStateException("HmacSHA256 unavailable", e);
        }
    }
}
