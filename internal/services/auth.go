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

type AuthService struct {
	repo         *repository.Repository
	tokens       *TokenService
	clientID     string
	clientSecret string
	redirectURL  string
}

func NewAuthService(repo *repository.Repository, tokens *TokenService, clientID, clientSecret, redirectURL string) *AuthService {
	return &AuthService{repo: repo, tokens: tokens, clientID: clientID, clientSecret: clientSecret, redirectURL: redirectURL}
}

func (a *AuthService) GitHubAuthURL(state, codeChallenge string) string {
	params := url.Values{}
	params.Set("client_id", a.clientID)
	params.Set("redirect_uri", a.redirectURL)
	params.Set("state", state)
	params.Set("scope", "read:user user:email")
	if codeChallenge != "" {
		params.Set("code_challenge", codeChallenge)
		params.Set("code_challenge_method", "S256")
	}
	return "https://github.com/login/oauth/authorize?" + params.Encode()
}

func (a *AuthService) ExchangeCodeForUser(ctx context.Context, code, codeVerifier string) (*models.User, error) {
	githubToken, err := a.exchangeCodeForGitHubToken(ctx, code, codeVerifier)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	ghUser, err := a.fetchGitHubUser(ctx, githubToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch github user: %w", err)
	}
	return &models.User{
		GithubID:  fmt.Sprintf("%d", ghUser.ID),
		Username:  ghUser.Login,
		Email:     ghUser.Email,
		AvatarURL: ghUser.AvatarURL,
		Role:      "analyst",
	}, nil
}

func (a *AuthService) IssueTokenPair(ctx context.Context, user *models.User) (accessToken, refreshToken string, err error) {
	saved, err := a.repo.UpsertUser(ctx, user)
	if err != nil {
		return "", "", fmt.Errorf("upsert user: %w", err)
	}
	accessToken, err = a.tokens.IssueAccessToken(saved.ID, saved.Role)
	if err != nil {
		return "", "", fmt.Errorf("issue access token: %w", err)
	}
	refreshToken, err = a.tokens.IssueRefreshToken()
	if err != nil {
		return "", "", fmt.Errorf("issue refresh token: %w", err)
	}
	hash := HashToken(refreshToken)
	if err = a.repo.StoreRefreshToken(ctx, saved.ID, hash, time.Now().Add(RefreshTokenDuration)); err != nil {
		return "", "", fmt.Errorf("store refresh token: %w", err)
	}
	return accessToken, refreshToken, nil
}

func (a *AuthService) RefreshTokenPair(ctx context.Context, rawRefreshToken string) (accessToken, refreshToken string, err error) {
	hash := HashToken(rawRefreshToken)
	stored, err := a.repo.GetRefreshToken(ctx, hash)
	if err != nil {
		return "", "", fmt.Errorf("lookup refresh token: %w", err)
	}
	if stored == nil {
		return "", "", fmt.Errorf("refresh token invalid or expired")
	}
	if err = a.repo.DeleteRefreshToken(ctx, hash); err != nil {
		return "", "", fmt.Errorf("delete old refresh token: %w", err)
	}
	user, err := a.repo.GetUserByID(ctx, stored.UserID)
	if err != nil || user == nil {
		return "", "", fmt.Errorf("user not found")
	}
	accessToken, err = a.tokens.IssueAccessToken(user.ID, user.Role)
	if err != nil {
		return "", "", err
	}
	refreshToken, err = a.tokens.IssueRefreshToken()
	if err != nil {
		return "", "", err
	}
	if err = a.repo.StoreRefreshToken(ctx, user.ID, HashToken(refreshToken), time.Now().Add(RefreshTokenDuration)); err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

func (a *AuthService) RevokeUserTokens(ctx context.Context, userID string) error {
	return a.repo.DeleteUserRefreshTokens(ctx, userID)
}

type githubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

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
