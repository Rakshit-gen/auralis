// Package authn handles JWT verification and role-based authorization.
//
// The gateway verifies the caller's access token and forwards identity to
// downstream services as signed headers (X-Auralis-User, X-Auralis-Roles,
// X-Auralis-Request-Id) plus an HMAC over them in X-Auralis-Identity-Sig so a
// downstream service can trust the headers without re-parsing the JWT. Each
// domain service still performs its own authorization checks.
package authn

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/auralis/platform/errcodes"
	"github.com/golang-jwt/jwt/v5"
)

// Role values recognised across the platform.
const (
	RoleUser    = "USER"
	RoleCreator = "CREATOR"
	RoleAdmin   = "ADMIN"
)

// Identity is the authenticated caller attached to a request context.
type Identity struct {
	UserID string
	Roles  []string
}

// HasRole reports whether the identity holds role.
func (i Identity) HasRole(role string) bool { return slices.Contains(i.Roles, role) }

// HasAny reports whether the identity holds any of roles.
func (i Identity) HasAny(roles ...string) bool {
	for _, r := range roles {
		if i.HasRole(r) {
			return true
		}
	}
	return false
}

type ctxKey int

const identityKey ctxKey = 0

// Claims is the JWT body issued by the auth service.
type Claims struct {
	Roles []string `json:"roles"`
	Type  string   `json:"typ"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// Verifier validates access tokens with a shared HMAC secret.
type Verifier struct {
	secret   []byte
	issuer   string
	audience string
}

func NewVerifier(secret, issuer, audience string) *Verifier {
	return &Verifier{secret: []byte(secret), issuer: issuer, audience: audience}
}

// Parse verifies token and returns its claims. Only "access" tokens are accepted.
func (v *Verifier) Parse(token string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(token, c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errcodes.New(http.StatusUnauthorized, errcodes.InvalidToken, "unexpected signing method")
		}
		return v.secret, nil
	}, jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired())
	if err != nil {
		return nil, errcodes.New(http.StatusUnauthorized, errcodes.InvalidToken, "invalid or expired token")
	}
	if c.Type != "access" {
		return nil, errcodes.New(http.StatusUnauthorized, errcodes.InvalidToken, "not an access token")
	}
	return c, nil
}

// IdentityHeaders signs and returns the downstream identity headers.
func IdentityHeaders(secret string, id Identity, requestID string) map[string]string {
	roles := strings.Join(id.Roles, ",")
	sig := signIdentity(secret, id.UserID, roles, requestID)
	return map[string]string{
		"X-Auralis-User":         id.UserID,
		"X-Auralis-Roles":        roles,
		"X-Auralis-Request-Id":   requestID,
		"X-Auralis-Identity-Sig": sig,
	}
}

func signIdentity(secret, user, roles, requestID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(user + "\n" + roles + "\n" + requestID))
	return hex.EncodeToString(mac.Sum(nil))
}

// FromHeaders validates the signed identity headers set by the gateway and
// returns the caller identity. Services use this instead of parsing JWTs.
func FromHeaders(secret string, h http.Header) (Identity, string, error) {
	user := h.Get("X-Auralis-User")
	roles := h.Get("X-Auralis-Roles")
	requestID := h.Get("X-Auralis-Request-Id")
	sig := h.Get("X-Auralis-Identity-Sig")
	if user == "" || sig == "" {
		return Identity{}, requestID, errcodes.Unauthed("missing identity headers")
	}
	want := signIdentity(secret, user, roles, requestID)
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return Identity{}, requestID, errcodes.Unauthed("identity signature mismatch")
	}
	id := Identity{UserID: user}
	if roles != "" {
		id.Roles = strings.Split(roles, ",")
	}
	return id, requestID, nil
}

// WithIdentity stores id on ctx.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// FromContext returns the caller identity, and false if the request is anonymous.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}

// MustIdentity returns the caller identity or an unauthorized error.
func MustIdentity(ctx context.Context) (Identity, error) {
	id, ok := FromContext(ctx)
	if !ok || id.UserID == "" {
		return Identity{}, errcodes.Unauthed("authentication required")
	}
	return id, nil
}

// ServiceMiddleware validates gateway identity headers on every request. Routes
// that allow anonymous access should be mounted before this or check inside the
// handler; the gateway only forwards headers for authenticated calls.
func ServiceMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Auralis-User") != "" {
				id, _, err := FromHeaders(secret, r.Header)
				if err != nil {
					errcodes.Write(w, r.Header.Get("X-Auralis-Request-Id"), err)
					return
				}
				r = r.WithContext(WithIdentity(r.Context(), id))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRoles returns middleware that rejects callers lacking any of roles.
func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := MustIdentity(r.Context())
			if err != nil {
				errcodes.Write(w, r.Header.Get("X-Auralis-Request-Id"), err)
				return
			}
			if len(roles) > 0 && !id.HasAny(roles...) {
				errcodes.Write(w, r.Header.Get("X-Auralis-Request-Id"), errcodes.Denied("insufficient role"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AccessTTL and RefreshTTL are the platform defaults; the auth service owns issuance.
const (
	AccessTTL  = 15 * time.Minute
	RefreshTTL = 30 * 24 * time.Hour
)
