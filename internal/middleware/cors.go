package middleware

import (
	"net/http"
)

// CORS adds Cross-Origin Resource Sharing headers so the web portal
// (hosted on a different origin than the backend) can make credentialed
// requests (requests that include HTTP-only cookies).
//
// Key rules:
//   - Access-Control-Allow-Origin must be the *exact* portal origin, not "*",
//     when credentials are involved. Browsers refuse wildcard + credentials.
//   - Access-Control-Allow-Credentials: true tells the browser it's OK to
//     include cookies on this cross-origin request.
//   - We echo the request Origin only when it matches our allowed list,
//     so other origins still get rejected.
func CORS(allowedOrigins ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Version, X-CSRF-Token")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				// Vary tells CDN/proxy caches that this response varies by Origin.
				w.Header().Add("Vary", "Origin")
			}

			// Handle preflight (OPTIONS) requests — the browser sends these
			// before any cross-origin request with custom headers to check
			// whether the server will accept it.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
