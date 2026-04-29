package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessTokenDuration is intentionally very short.
// If an access token is stolen, the attacker's window is tiny.
const AccessTokenDuration = 3 * time.Minute

// RefreshTokenDuration is longer — it's the "remember me" lifetime.
// On each use the old one is deleted and a new pair is issued (rotation).
const RefreshTokenDuration = 5 * time.Minute

// Claims is the payload we embed inside the access token JWT.
// We keep it minimal: only what middleware needs to make auth/RBAC decisions
// without hitting the database on every request.
type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	// jwt.RegisteredClaims carries standard fields: Issuer, Subject, ExpiresAt, IssuedAt, etc.
	jwt.RegisteredClaims
}

// TokenService handles issuing and validating JWTs.
// It holds the two secrets as byte slices (the format jwt library expects).
type TokenService struct {
	accessSecret  []byte
	refreshSecret []byte
}

func NewTokenService(accessSecret, refreshSecret string) *TokenService {
	return &TokenService{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
	}
}

// IssueAccessToken signs a short-lived JWT containing the user's ID and role.
// The token is signed with HMAC-SHA256 using the access secret.
func (t *TokenService) IssueAccessToken(userID, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			// ExpiresAt tells jwt library when to reject this token automatically.
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   userID,
		},
	}

	// jwt.NewWithClaims creates an unsigned token object.
	// SignedString(secret) computes the signature and returns the full JWT string.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(t.accessSecret)
}

// ValidateAccessToken parses and verifies an access token.
// It returns the embedded claims so callers know who the user is and their role.
func (t *TokenService) ValidateAccessToken(tokenStr string) (*Claims, error) {
	return t.parseToken(tokenStr, t.accessSecret)
}

// IssueRefreshToken generates a cryptographically random opaque token.
// Unlike access tokens, refresh tokens are NOT JWTs — they are random bytes
// stored (hashed) in the DB. The user ID is found by looking up the hash.
func (t *TokenService) IssueRefreshToken() (rawToken string, err error) {
	// 32 random bytes = 256 bits of entropy. Impossible to brute-force.
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	// base64url encoding makes it safe to send in JSON / HTTP headers.
	return base64.URLEncoding.EncodeToString(b), nil
}

// HashToken produces a SHA-256 hex digest of the token.
// This is what we store in the database instead of the raw token.
// If the DB is leaked, hashes are useless without the original token.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// parseToken is the shared validation logic for both token types.
// The keyFunc callback is how the jwt library asks us "which key should I
// use to verify THIS token?" — we always return the same secret, but the
// pattern allows per-token-type key selection.
func (t *TokenService) parseToken(tokenStr string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		// Guard against algorithm confusion attacks: ensure the token was
		// signed with HMAC (not RSA or ECDSA), so an attacker can't swap
		// the algorithm header to bypass verification.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}
