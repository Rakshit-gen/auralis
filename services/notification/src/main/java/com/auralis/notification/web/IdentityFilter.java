package com.auralis.notification.web;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.io.IOException;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;
import tools.jackson.databind.ObjectMapper;

/**
 * Requires a valid gateway identity on every /notifications request and stores
 * it as a request attribute. Health and metrics stay open, as on the Go
 * services, because Render and Prometheus call them directly.
 */
@Component
public class IdentityFilter extends OncePerRequestFilter {

    private final String secret;
    private final ObjectMapper json;

    /** Same rule as the Go config.RequireSecret and the Python settings. */
    static final int MIN_SECRET_LEN = 32;

    public IdentityFilter(
            @Value("${IDENTITY_SECRET}") String secret,
            @Value("${ALLOW_DEV_SECRETS:false}") boolean allowDevSecrets,
            ObjectMapper json) {
        checkSecret(secret, allowDevSecrets);
        this.secret = secret;
        this.json = json;
    }

    static void checkSecret(String secret, boolean allowDevSecrets) {
        if (secret.isBlank()) {
            throw new IllegalStateException("IDENTITY_SECRET is required");
        }
        if (secret.length() < MIN_SECRET_LEN) {
            throw new IllegalStateException("IDENTITY_SECRET is shorter than " + MIN_SECRET_LEN + " characters");
        }
        if (secret.toLowerCase().contains("change-me") && !allowDevSecrets) {
            throw new IllegalStateException("IDENTITY_SECRET is still the example value; set a real secret");
        }
    }

    @Override
    protected boolean shouldNotFilter(HttpServletRequest req) {
        String path = req.getRequestURI();
        return !(path.equals("/notifications") || path.startsWith("/notifications/"));
    }

    @Override
    protected void doFilterInternal(HttpServletRequest req, HttpServletResponse res, FilterChain chain)
            throws ServletException, IOException {
        Identity id;
        try {
            id = Identity.verify(secret,
                    req.getHeader("X-Auralis-User"),
                    req.getHeader("X-Auralis-Roles"),
                    req.getHeader("X-Auralis-Request-Id"),
                    req.getHeader("X-Auralis-Issued-At"),
                    req.getHeader("X-Auralis-Identity-Sig"));
        } catch (ApiException e) {
            res.setStatus(e.status().value());
            res.setContentType(MediaType.APPLICATION_JSON_VALUE);
            json.writeValue(res.getOutputStream(),
                    ErrorBody.of(e.code(), e.getMessage(), req.getHeader("X-Auralis-Request-Id")));
            return;
        }
        req.setAttribute(Identity.ATTRIBUTE, id);
        chain.doFilter(req, res);
    }
}
