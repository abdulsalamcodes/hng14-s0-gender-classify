package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"hng14-s0-gender-classify/internal/middleware"
	"hng14-s0-gender-classify/internal/services"
)

type pendingState struct {
	codeChallenge  string
	source         string
	cliRedirectURI string
	expiresAt      time.Time
}

var (
	stateStore   = &sync.Map{}
	stateCleanup = time.NewTicker(5 * time.Minute)
)

func init() {
	go func() {
		for range stateCleanup.C {
			stateStore.Range(func(k, v any) bool {
				if ps, ok := v.(pendingState); ok && time.Now().After(ps.expiresAt) {
					stateStore.Delete(k)
				}
				return true
			})
		}
	}()
}

type AuthHandler struct {
	auth        *services.AuthService
	tokens      *services.TokenService
	frontendURL string
}

func NewAuthHandler(auth *services.AuthService, tokens *services.TokenService, frontendURL string) *AuthHandler {
	return &AuthHandler{auth: auth, tokens: tokens, frontendURL: frontendURL}
}

func (h *AuthHandler) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		writeError(w, "failed to generate state", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(stateBytes)

	source := r.URL.Query().Get("source")
	codeChallenge := r.URL.Query().Get("code_challenge")
	cliRedirectURI := r.URL.Query().Get("cli_redirect_uri")

	stateStore.Store(state, pendingState{
		codeChallenge:  codeChallenge,
		source:         source,
		cliRedirectURI: cliRedirectURI,
		expiresAt:      time.Now().Add(10 * time.Minute),
	})

	http.Redirect(w, r, h.auth.GitHubAuthURL(state, codeChallenge), http.StatusTemporaryRedirect)
}

func (h *AuthHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		writeError(w, "github oauth denied: "+errParam, http.StatusUnauthorized)
		return
	}

	val, ok := stateStore.LoadAndDelete(state)
	if !ok {
		writeError(w, "invalid or expired state", http.StatusBadRequest)
		return
	}
	ps := val.(pendingState)
	if time.Now().After(ps.expiresAt) {
		writeError(w, "state expired", http.StatusBadRequest)
		return
	}

	if ps.source == "cli" {
		if ps.cliRedirectURI == "" {
			writeError(w, "cli_redirect_uri missing from state", http.StatusBadRequest)
			return
		}
		sep := "?"
		for _, c := range ps.cliRedirectURI {
			if c == '?' {
				sep = "&"
				break
			}
		}
		http.Redirect(w, r, ps.cliRedirectURI+sep+"code="+code+"&state="+state, http.StatusTemporaryRedirect)
		return
	}

	user, err := h.auth.ExchangeCodeForUser(r.Context(), code, "")
	if err != nil {
		writeError(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	accessToken, refreshToken, err := h.auth.IssueTokenPair(r.Context(), user)
	if err != nil {
		writeError(w, "failed to issue tokens", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(services.AccessTokenDuration.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(services.RefreshTokenDuration.Seconds()),
	})

	csrfBytes := make([]byte, 16)
	rand.Read(csrfBytes) //nolint:errcheck
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    hex.EncodeToString(csrfBytes),
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(services.AccessTokenDuration.Seconds()),
	})

	http.Redirect(w, r, h.frontendURL+"/#dashboard", http.StatusTemporaryRedirect)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}

	cookie, cookieErr := r.Cookie("refresh_token")
	if cookieErr == nil && cookie.Value != "" {
		body.RefreshToken = cookie.Value
	} else {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RefreshToken == "" {
			writeError(w, "refresh_token required", http.StatusBadRequest)
			return
		}
	}

	accessToken, refreshToken, err := h.auth.RefreshTokenPair(r.Context(), body.RefreshToken)
	if err != nil {
		writeError(w, "invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	if cookieErr == nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "access_token",
			Value:    accessToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(services.AccessTokenDuration.Seconds()),
		})
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    refreshToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(services.RefreshTokenDuration.Seconds()),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":        "success",
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func (h *AuthHandler) CLIExchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code         string `json:"code"`
		CodeVerifier string `json:"code_verifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" || body.CodeVerifier == "" {
		writeError(w, "code and code_verifier required", http.StatusBadRequest)
		return
	}

	user, err := h.auth.ExchangeCodeForUser(r.Context(), body.Code, body.CodeVerifier)
	if err != nil {
		writeError(w, "token exchange failed", http.StatusUnauthorized)
		return
	}

	accessToken, refreshToken, err := h.auth.IssueTokenPair(r.Context(), user)
	if err != nil {
		writeError(w, "failed to issue tokens", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":        "success",
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"username":      user.Username,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if claims := middleware.ClaimsFromContext(r.Context()); claims != nil {
		_ = h.auth.RevokeUserTokens(r.Context(), claims.UserID)
	}

	http.SetCookie(w, &http.Cookie{Name: "access_token", Value: "", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", MaxAge: -1, Path: "/"})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "logged out"})
}
