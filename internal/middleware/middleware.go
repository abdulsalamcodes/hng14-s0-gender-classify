package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"hng14-s0-gender-classify/internal/services"
)

type contextKey string

const claimsContextKey contextKey = "claims"

func ClaimsFromContext(ctx context.Context) *services.Claims {
	val := ctx.Value(claimsContextKey)
	if val == nil {
		return nil
	}
	claims, ok := val.(*services.Claims)
	if !ok {
		return nil
	}
	return claims
}

func AuthMiddleware(tokenService *services.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string

			cookie, err := r.Cookie("access_token")
			if err == nil && cookie.Value != "" {
				tokenStr = cookie.Value
			}

			if tokenStr == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if tokenStr == "" {
				http.Error(w, `{"status":"error","message":"missing access token"}`, http.StatusUnauthorized)
				return
			}

			claims, err := tokenService.ValidateAccessToken(tokenStr)
			if err != nil {
				http.Error(w, `{"status":"error","message":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RBACMiddleware(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, `{"status":"error","message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if claims.Role != requiredRole {
				http.Error(w, `{"status":"error","message":"forbidden"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func APIVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Version") != "1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "API version header required",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

