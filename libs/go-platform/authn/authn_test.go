package authn

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-value-please-change"

func issue(t *testing.T, typ string, roles []string, ttl time.Duration) string {
	t.Helper()
	c := Claims{
		Roles: roles,
		Type:  typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			Issuer:    "auralis-auth",
			Audience:  jwt.ClaimStrings{"auralis"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestVerifierAcceptsAccessRejectsRefresh(t *testing.T) {
	v := NewVerifier(testSecret, "auralis-auth", "auralis")

	claims, err := v.Parse(issue(t, "access", []string{RoleUser}, time.Minute))
	if err != nil {
		t.Fatalf("valid access token rejected: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("wrong subject: %s", claims.Subject)
	}

	if _, err := v.Parse(issue(t, "refresh", nil, time.Minute)); err == nil {
		t.Fatal("refresh token accepted as access token")
	}
	if _, err := v.Parse(issue(t, "access", nil, -time.Minute)); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestIdentityHeaderRoundTripAndTamper(t *testing.T) {
	id := Identity{UserID: "user-9", Roles: []string{RoleUser, RoleCreator}}
	h := http.Header{}
	for k, val := range IdentityHeaders(testSecret, id, "req-1") {
		h.Set(k, val)
	}

	got, reqID, err := FromHeaders(testSecret, h)
	if err != nil {
		t.Fatalf("valid headers rejected: %v", err)
	}
	if reqID != "req-1" || got.UserID != "user-9" || !got.HasRole(RoleCreator) {
		t.Fatalf("identity not reconstructed: %+v", got)
	}

	h.Set("X-Auralis-Roles", "USER,CREATOR,ADMIN") // privilege escalation attempt
	if _, _, err := FromHeaders(testSecret, h); err == nil {
		t.Fatal("tampered roles header accepted")
	}
}
