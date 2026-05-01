package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"hng14-s0-gender-classify/internal/repository"
	"hng14-s0-gender-classify/internal/services"
)

type contextKey string

const claimsContextKey contextKey = "claims"

func ClaimsFromContext(ctx context.Context) *services.Claims {
	c, _ := ctx.Value(claimsContextKey).(*services.Claims)
	return c
}

func RequireAuth(tokens *services.TokenService, repo *repository.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				writeJSONError(w, "authentication required", http.StatusUnauthorized)
				return
			}

			claims, err := tokens.ValidateAccessToken(tokenStr)
			if err != nil {
				writeJSONError(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			user, err := repo.GetUserByID(r.Context(), claims.UserID)
			if err != nil || user == nil {
				writeJSONError(w, "user not found", http.StatusUnauthorized)
				return
			}
			if !user.IsActive {
				writeJSONError(w, "account is deactivated", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				writeJSONError(w, "authentication required", http.StatusUnauthorized)
				return
			}
			if !allowed[claims.Role] {
				writeJSONError(w, "insufficient permissions", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func APIVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Version") != "1" {
			writeJSONError(w, "API version header required", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if parts := strings.SplitN(h, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return parts[1]
		}
	}
	if cookie, err := r.Cookie("access_token"); err == nil {
		return cookie.Value
	}
	return ""
}

func writeJSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": message})
}
