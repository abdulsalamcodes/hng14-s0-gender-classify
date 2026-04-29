package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hng14-s0-gender-classify/internal/models"
	"hng14-s0-gender-classify/internal/repository"
)

// AuthService handles everything OAuth + token related.
// It coordinates between the GitHub API, our DB (via repo), and the token service.
type AuthService struct {
	repo         *repository.Repository
	tokens       *TokenService
	clientID     string
	clientSecret string
	redirectURL  string
}

func NewAuthService(repo *repository.Repository, tokens *TokenService, clientID, clientSecret, redirectURL string) *AuthService {
	return &AuthService{
		repo:         repo,
		tokens:       tokens,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
	}
}

// GitHubAuthURL builds the URL we send the user to on GitHub.
// Parameters:
//   - state: random string for CSRF protection (generated per-login by the handler)
//   - codeChallenge: the PKCE challenge (SHA256 of code_verifier). Empty string for plain web flow.
//
// The user sees GitHub's login page, approves access, and GitHub redirects them
// to our callback URL with ?code=...&state=...
func (a *AuthService) GitHubAuthURL(state, codeChallenge string) string {
	params := url.Values{}
	params.Set("client_id", a.clientID)
	params.Set("redirect_uri", a.redirectURL)
	params.Set("state", state)
	params.Set("scope", "read:user user:email")

	// Only add PKCE parameters when the CLI provides a challenge.
	// The web flow uses the server-side client_secret instead.
	if codeChallenge != "" {
		params.Set("code_challenge", codeChallenge)
		params.Set("code_challenge_method", "S256")
	}

	return "https://github.com/login/oauth/authorize?" + params.Encode()
}

// ExchangeCodeForUser is called after GitHub redirects back to our callback.
// It takes the one-time `code` from GitHub and exchanges it for a GitHub access token,
// then uses that token to fetch the user's profile.
//
// codeVerifier is the PKCE secret sent by the CLI. For web logins it can be "".
func (a *AuthService) ExchangeCodeForUser(ctx context.Context, code, codeVerifier string) (*models.User, error) {
	// Step 1: exchange the authorization code for a GitHub access token.
	githubToken, err := a.exchangeCodeForGitHubToken(ctx, code, codeVerifier)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// Step 2: use the GitHub access token to fetch the user's profile.
	ghUser, err := a.fetchGitHubUser(ctx, githubToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch github user: %w", err)
	}

	// Map GitHub's user shape to our internal User model.
	// Role defaults to "analyst" — admins must be promoted manually in the DB.
	user := &models.User{
		GithubID:  fmt.Sprintf("%d", ghUser.ID),
		Username:  ghUser.Login,
		Email:     ghUser.Email,
		AvatarURL: ghUser.AvatarURL,
		Role:      "analyst",
	}
	return user, nil
}

// IssueTokenPair persists the user (upsert) and issues a fresh access + refresh token pair.
// This is called at the end of every successful OAuth flow.
func (a *AuthService) IssueTokenPair(ctx context.Context, user *models.User) (accessToken, refreshToken string, err error) {
	// Persist user (creates or updates existing record).
	saved, err := a.repo.UpsertUser(ctx, user)
	if err != nil {
		return "", "", fmt.Errorf("upsert user: %w", err)
	}

	// Issue access token (JWT, 3 min).
	accessToken, err = a.tokens.IssueAccessToken(saved.ID, saved.Role)
	if err != nil {
		return "", "", fmt.Errorf("issue access token: %w", err)
	}

	// Issue refresh token (random bytes, 5 min).
	refreshToken, err = a.tokens.IssueRefreshToken()
	if err != nil {
		return "", "", fmt.Errorf("issue refresh token: %w", err)
	}

	// Hash and store the refresh token. We record when it expires so the DB
	// can serve as a secondary expiry check beyond the token's own claims.
	hash := HashToken(refreshToken)
	expiresAt := time.Now().Add(RefreshTokenDuration)
	if err = a.repo.StoreRefreshToken(ctx, saved.ID, hash, expiresAt); err != nil {
		return "", "", fmt.Errorf("store refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

// RefreshTokenPair validates an existing refresh token, rotates it
// (old one deleted, new pair issued), and returns the new tokens.
//
// Token rotation is a security best practice: each refresh token can only be
// used once. If someone steals a refresh token and uses it before you do,
// your next refresh attempt fails — alerting you that the token was stolen.
func (a *AuthService) RefreshTokenPair(ctx context.Context, rawRefreshToken string) (accessToken, refreshToken string, err error) {
	hash := HashToken(rawRefreshToken)

	// Look up the stored token. GetRefreshToken also checks expiry in SQL.
	stored, err := a.repo.GetRefreshToken(ctx, hash)
	if err != nil {
		return "", "", fmt.Errorf("lookup refresh token: %w", err)
	}
	if stored == nil {
		return "", "", fmt.Errorf("refresh token invalid or expired")
	}

	// Immediately delete the old token before issuing a new pair.
	// This is the "rotation" — even if this function crashes after deletion,
	// the old token is gone and the user will need to log in again (safe).
	if err = a.repo.DeleteRefreshToken(ctx, hash); err != nil {
		return "", "", fmt.Errorf("delete old refresh token: %w", err)
	}

	// Load the user to get their current role (role may have changed since last login).
	user, err := a.repo.GetUserByID(ctx, stored.UserID)
	if err != nil || user == nil {
		return "", "", fmt.Errorf("user not found")
	}

	// Issue a fresh pair.
	accessToken, err = a.tokens.IssueAccessToken(user.ID, user.Role)
	if err != nil {
		return "", "", err
	}
	refreshToken, err = a.tokens.IssueRefreshToken()
	if err != nil {
		return "", "", err
	}
	newHash := HashToken(refreshToken)
	if err = a.repo.StoreRefreshToken(ctx, user.ID, newHash, time.Now().Add(RefreshTokenDuration)); err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

// RevokeUserTokens deletes all refresh tokens for a user — used on logout.
func (a *AuthService) RevokeUserTokens(ctx context.Context, userID string) error {
	return a.repo.DeleteUserRefreshTokens(ctx, userID)
}

// ── GitHub API helpers ────────────────────────────────────────────────────────

// githubUser is the subset of fields we care about from the GitHub /user API.
type githubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// exchangeCodeForGitHubToken calls GitHub's token endpoint to swap the
// one-time authorization code for a GitHub OAuth access token.
func (a *AuthService) exchangeCodeForGitHubToken(ctx context.Context, code, codeVerifier string) (string, error) {
	body := url.Values{}
	body.Set("client_id", a.clientID)
	body.Set("client_secret", a.clientSecret)
	body.Set("code", code)
	body.Set("redirect_uri", a.redirectURL)
	if codeVerifier != "" {
		body.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Tell GitHub to return JSON instead of the default URL-encoded response.
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("github oauth error: %s — %s", result.Error, result.ErrorDesc)
	}
	return result.AccessToken, nil
}

// fetchGitHubUser calls the GitHub API to get the authenticated user's profile.
func (a *AuthService) fetchGitHubUser(ctx context.Context, githubAccessToken string) (*githubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+githubAccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github /user returned %d: %s", resp.StatusCode, b)
	}

	var user githubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	if user.Login == "" {
		return nil, fmt.Errorf("github returned empty user")
	}
	return &user, nil
}
