package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

// AuthRateLimit limits /auth/* endpoints to 10 requests per minute per IP.
//
// Why per-IP for auth routes? The user isn't authenticated yet, so there's
// no user ID available. IP-based limiting prevents brute-forcing the OAuth
// flow or hammering the token endpoints.
func AuthRateLimit() func(http.Handler) http.Handler {
	return httprate.Limit(
		10,
		time.Minute,
		httprate.WithKeyFuncs(httprate.KeyByRealIP),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writeJSONError(w, "too many requests, please slow down", http.StatusTooManyRequests)
		}),
	)
}

// APIRateLimit limits /api/* endpoints to 60 requests per minute per user.
//
// Why per-user (not per-IP)? Many users may share a public IP (office NAT,
// university network). Keying by user ID is fairer and harder to abuse —
// an attacker would need a valid token to get extra quota.
func APIRateLimit() func(http.Handler) http.Handler {
	return httprate.Limit(
		60,
		time.Minute,
		httprate.WithKeyFuncs(
			func(r *http.Request) (string, error) {
				// Use user ID from JWT claims when available (authenticated requests).
				if claims := ClaimsFromContext(r.Context()); claims != nil {
					return "uid:" + claims.UserID, nil
				}
				// Fallback to IP for any unauthenticated request that slips through.
				return httprate.KeyByRealIP(r)
			},
		),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writeJSONError(w, "rate limit exceeded", http.StatusTooManyRequests)
		}),
	)
}
