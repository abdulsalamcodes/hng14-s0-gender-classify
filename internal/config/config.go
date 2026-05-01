package config

import (
	"os"
)

type Config struct {
	DatabaseURL string

	GenderizeURL   string
	AgifyURL       string
	NationalizeURL string


	GitHubClientID     string
	GitHubClientSecret string
	GitHubRedirectURL  string

	JWTSecret        string
	JWTRefreshSecret string

	FrontendURL string
}

func Load() *Config {
	return &Config{
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"),
		GenderizeURL:   getEnv("GENDERIZE_URL", "https://api.genderize.io"),
		AgifyURL:       getEnv("AGIFY_URL", "https://api.agify.io"),
		NationalizeURL: getEnv("NATIONALIZE_URL", "https://api.nationalize.io"),

		GitHubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
		GitHubRedirectURL:  getEnv("GITHUB_REDIRECT_URL", "http://localhost:8080/auth/github/callback"),

		JWTSecret:        getEnv("JWT_SECRET", "change-me-access-secret"),
		JWTRefreshSecret: getEnv("JWT_REFRESH_SECRET", "change-me-refresh-secret"),

		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
