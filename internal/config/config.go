package config

import (
	"os"
)

type Config struct {
	DatabaseURL     string
	GenderizeURL    string
	AgifyURL        string
	NationalizeURL  string
}

func Load() *Config {
	return &Config{
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"),
		GenderizeURL:   getEnv("GENDERIZE_URL", "https://api.genderize.io"),
		AgifyURL:       getEnv("AGIFY_URL", "https://api.agify.io"),
		NationalizeURL: getEnv("NATIONALIZE_URL", "https://api.nationalize.io"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
