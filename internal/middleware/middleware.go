package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"hng14-s0-gender-classify/internal/repository"
	"hng14-s0-gender-classify/internal/services"
)

// contextKey is an unexported type for context keys to prevent collisions
// with any other package that might store values in the same context.
type contextKey string

const claimsContextKey contextKey = "claims"

// ClaimsFromContext retrieves JWT claims attached by RequireAuth/AuthMiddleware.
// Returns nil on unauthenticated routes.
func ClaimsFromContext(ctx context.Context) *services.Claims {
	c, _ := ctx.Value(claimsContextKey).(*services.Claims)
	return c
}

// RequireAuth validates the Bearer token (or access_token cookie) on every
// protected request. On success it attaches Claims to the context.
//
// Token sources checked (in order):
//  1. Authorization: Bearer <token>  — CLI and direct API calls
//  2. access_token cookie            — Web portal (HTTP-only, set at login)
//
// It also checks is_active in the DB so a deactivated account is immediately
// blocked, even within the 3-minute access token window.
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

			// Check is_active via DB. Access tokens are short-lived (3 min)
			// so this only fires on actual requests, not on every clock tick.
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

// RequireRole allows only users whose role is in the provided list.
// Must be used after RequireAuth (which populates context with claims).
//
//	r.With(RequireRole("admin")).Post("/api/profiles", h.CreateProfile)
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

// APIVersionMiddleware rejects requests that don't carry X-API-Version: 1.
// Applied to the entire /api/* router group.
func APIVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Version") != "1" {
			writeJSONError(w, "API version header required", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractToken reads the Bearer token from the Authorization header,
// or falls back to the access_token HTTP-only cookie (web portal).
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

// writeJSONError writes a standard { "status": "error", "message": "..." } response.
// Kept here to make the middleware package self-contained (avoids import cycles
// with the handlers package).
func writeJSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": message})
}
