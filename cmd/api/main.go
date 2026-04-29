package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"hng14-s0-gender-classify/internal/config"
	"hng14-s0-gender-classify/internal/handlers"
	"hng14-s0-gender-classify/internal/middleware"
	"hng14-s0-gender-classify/internal/repository"
	"hng14-s0-gender-classify/internal/services"
	"hng14-s0-gender-classify/pkg/api"
)

func main() {
	godotenv.Load()

	cfg := config.Load()

	ctx := context.Background()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	repo := repository.New(db)
	if err := repo.InitSchema(ctx); err != nil {
		log.Fatalf("Failed to init schema: %v", err)
	}

	apiClient := api.NewClient(cfg.GenderizeURL, cfg.AgifyURL, cfg.NationalizeURL)
	svc := services.New(repo, apiClient)

	tokenSvc := services.NewTokenService(cfg.JWTSecret, cfg.JWTRefreshSecret)
	authSvc := services.NewAuthService(repo, tokenSvc, cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.GitHubRedirectURL)
	authHandler := handlers.NewAuthHandler(authSvc, tokenSvc, cfg.FrontendURL)

	h := handlers.New(svc)

	r := chi.NewRouter()

	r.Use(middleware.LoggingMiddleware)
	r.Use(middleware.RateLimitMiddleware)
	r.Use(chimiddleware.Recoverer)

	r.Get("/", h.Root)

	r.Route("/auth", func(r chi.Router) {
		r.Get("/github", authHandler.GitHubLogin)
		r.Get("/github/callback", authHandler.GitHubCallback)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/logout", authHandler.Logout)
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.APIVersionMiddleware)

		r.Get("/classify", h.Classify)

		r.With(middleware.AuthMiddleware(tokenSvc)).Route("/whoami", func(r chi.Router) {
			r.Get("/", h.Whoami)
		})

		r.Route("/profiles", func(r chi.Router) {
			r.Get("/", h.ListProfiles)
			r.Get("/search", h.SearchProfiles)
			r.Get("/export", h.ExportProfiles)
			r.Post("/", h.CreateProfile)
			r.Get("/{id}", h.GetProfile)
			r.Delete("/{id}", h.DeleteProfile)
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}