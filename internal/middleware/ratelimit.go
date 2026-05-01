package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

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

func APIRateLimit() func(http.Handler) http.Handler {
	return httprate.Limit(
		60,
		time.Minute,
		httprate.WithKeyFuncs(
			func(r *http.Request) (string, error) {
				if claims := ClaimsFromContext(r.Context()); claims != nil {
					return "uid:" + claims.UserID, nil
				}
				return httprate.KeyByRealIP(r)
			},
		),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writeJSONError(w, "rate limit exceeded", http.StatusTooManyRequests)
		}),
	)
}
