package internal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/auralis/platform/authn"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenConfig holds JWT signing parameters.
type TokenConfig struct {
	Secret     string
	Issuer     string
	Audience   string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// IssuedTokens is the pair returned to a client.
type IssuedTokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// signAccess builds a signed access JWT for a user with the given roles.
func (c TokenConfig) signAccess(userID string, roles []string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(c.AccessTTL)
	claims := authn.Claims{
		Roles: roles,
		Type:  "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    c.Issuer,
			Audience:  jwt.ClaimStrings{c.Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.NewString(),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(c.Secret))
	return tok, exp, err
}

// newOpaqueRefresh returns a random refresh token and its storage hash. The raw
// value goes to the client; only the hash is persisted.
func newOpaqueRefresh() (raw, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw)
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// newFamilyID returns an identifier for a fresh refresh-token chain.
func newFamilyID() string { return uuid.NewString() }
