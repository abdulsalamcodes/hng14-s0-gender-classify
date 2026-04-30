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


// pendingState stores PKCE and source info while the user is on GitHub's login page.
// We need to remember these between the initial redirect and the callback.
type pendingState struct {
	codeChallenge  string // sent by CLI; empty for web flow
	source         string // "cli" or "web"
	cliRedirectURI string // where the CLI's local server is listening (e.g. http://localhost:9876/callback)
	expiresAt      time.Time
}

// stateStore is an in-memory store for pending OAuth states.
// A sync.Map is used because the callback handler and the auth handler
// may run on different goroutines.
var (
	stateStore   = &sync.Map{}
	stateCleanup = time.NewTicker(5 * time.Minute) // periodically remove expired states
)

func init() {
	// Background goroutine to evict expired states so the map doesn't grow forever.
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

// AuthHandler groups all auth-related handlers.
// It embeds the auth service and token service so it can issue tokens.
type AuthHandler struct {
	auth        *services.AuthService
	tokens      *services.TokenService
	frontendURL string
}

func NewAuthHandler(auth *services.AuthService, tokens *services.TokenService, frontendURL string) *AuthHandler {
	return &AuthHandler{auth: auth, tokens: tokens, frontendURL: frontendURL}
}

// GitHubLogin initiates the OAuth flow.
// GET /auth/github?source=cli&code_challenge=<challenge>
//
// When the CLI calls this it passes:
//   - source=cli   (so we know to return JSON at the callback, not redirect)
//   - code_challenge=<sha256 of code_verifier>
//
// The web portal just links to /auth/github with no extra params.
func (h *AuthHandler) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	// Generate a cryptographically random state value.
	// 16 bytes = 128 bits of entropy → collision probability is negligible.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		writeError(w, "failed to generate state", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(stateBytes)

	source         := r.URL.Query().Get("source")           // "cli" or ""
	codeChallenge  := r.URL.Query().Get("code_challenge")   // provided by CLI (base64url SHA256 of verifier)
	cliRedirectURI := r.URL.Query().Get("cli_redirect_uri") // e.g. http://localhost:9876/callback

	// Store state info so the callback can retrieve it.
	stateStore.Store(state, pendingState{
		codeChallenge:  codeChallenge,
		source:         source,
		cliRedirectURI: cliRedirectURI,
		expiresAt:      time.Now().Add(10 * time.Minute),
	})

	redirectURL := h.auth.GitHubAuthURL(state, codeChallenge)
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// GitHubCallback handles the redirect from GitHub after the user approves login.
// GET /auth/github/callback?code=...&state=...
//
// Flow:
//  1. Validate state (CSRF check)
//  2. Exchange code + code_verifier with GitHub
//  3. Upsert user in DB
//  4. Issue access + refresh token pair
//  5. CLI → return JSON; Web → set HTTP-only cookies and redirect
func (h *AuthHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	// GitHub sends an error param when the user denies access.
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		writeError(w, "github oauth denied: "+errParam, http.StatusUnauthorized)
		return
	}

	// Validate state: look it up in the store, then immediately delete it
	// (states are single-use to prevent replay attacks).
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
		// CLI path: redirect the browser to the CLI's local callback server,
		// passing the code and state so the CLI can exchange them itself.
		//
		// Why not exchange here? The CLI holds code_verifier (the PKCE secret).
		// The backend never sees it — this keeps PKCE meaningful. The CLI calls
		// POST /auth/cli-exchange with {code, code_verifier, state} after
		// receiving the redirect.
		if ps.cliRedirectURI == "" {
			writeError(w, "cli_redirect_uri missing from state", http.StatusBadRequest)
			return
		}
		// Append code to the CLI's local server URL.
		sep := "?"
		if len(ps.cliRedirectURI) > 0 {
			for _, c := range ps.cliRedirectURI {
				if c == '?' {
					sep = "&"
					break
				}
			}
		}
		http.Redirect(w, r, ps.cliRedirectURI+sep+"code="+code+"&state="+state, http.StatusTemporaryRedirect)
		return
	}

	// Web path: exchange code here (server holds the client_secret; no PKCE verifier needed).
	user, err := h.auth.ExchangeCodeForUser(r.Context(), code, "")
	if err != nil {
		writeError(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	// Persist user + issue tokens.
	accessToken, refreshToken, err := h.auth.IssueTokenPair(r.Context(), user)
	if err != nil {
		writeError(w, "failed to issue tokens", http.StatusInternalServerError)
		return
	}

	// Web path: set HTTP-only cookies.
	// HttpOnly=true → JS cannot read these via document.cookie (XSS protection).
	// SameSite=Lax  → sent on same-site requests and top-level navigations (CSRF protection).
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

	// CSRF token: a readable (non-HttpOnly) cookie that JavaScript can read
	// and send as the X-CSRF-Token header on state-changing requests.
	// Because this token is also stored server-side via the signed access token,
	// an attacker on another origin cannot read it (SameSite + HttpOnly on the
	// main tokens ensures they can't forge requests with valid credentials).
	csrfBytes := make([]byte, 16)
	rand.Read(csrfBytes) //nolint:errcheck — rand.Read never errors
	csrfToken := hex.EncodeToString(csrfBytes)
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false, // Must be JS-readable so the portal can send it as a header.
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(services.AccessTokenDuration.Seconds()),
	})

	// Redirect to the web portal dashboard.
	http.Redirect(w, r, h.frontendURL+"/dashboard", http.StatusTemporaryRedirect)
}

// Refresh issues a new access + refresh token pair given a valid refresh token.
// POST /auth/refresh
// Body: { "refresh_token": "..." }
//
// The old refresh token is invalidated immediately (rotation).
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}

	// Support both JSON body (CLI/API) and cookie (web portal).
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

	// Update cookies for web clients.
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

// CLIExchange is called by the CLI after GitHub redirects back to its local server.
// POST /auth/cli-exchange
// Body: { "code": "...", "code_verifier": "..." }
//
// The CLI holds code_verifier (never sent to GitHub directly).
// Here the backend uses it to call GitHub's token endpoint and complete PKCE.
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

// Logout invalidates all refresh tokens for the calling user and clears cookies.
// POST /auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Get the user's ID from the JWT claims set by auth middleware.
	claims := middleware.ClaimsFromContext(r.Context())
	if claims != nil {
		// Delete all DB-stored refresh tokens for this user.
		_ = h.auth.RevokeUserTokens(r.Context(), claims.UserID)
	}

	// Clear cookies for web clients (set MaxAge=-1 to delete).
	http.SetCookie(w, &http.Cookie{Name: "access_token", Value: "", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", MaxAge: -1, Path: "/"})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "logged out",
	})
}
