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

	tokenSvc := services.NewTokenService(cfg.JWTSecret, cfg.JWTRefreshSecret)
	authSvc := services.NewAuthService(repo, tokenSvc, cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.GitHubRedirectURL)

	apiClient := api.NewClient(cfg.GenderizeURL, cfg.AgifyURL, cfg.NationalizeURL)
	profileSvc := services.New(repo, apiClient)

	authH := handlers.NewAuthHandler(authSvc, tokenSvc, cfg.FrontendURL)
	h := handlers.New(profileSvc)

	requireAuth := middleware.RequireAuth(tokenSvc, repo)
	requireAdmin := middleware.RequireRole("admin")

	r := chi.NewRouter()
	r.Use(middleware.CORS())
	r.Use(middleware.LoggingMiddleware)
	r.Use(chimiddleware.Recoverer)

	r.Get("/", h.Root)

	r.Route("/auth", func(r chi.Router) {
		r.Use(middleware.AuthRateLimit())
		r.Get("/github", authH.GitHubLogin)
		r.Get("/github/callback", authH.GitHubCallback)
		r.Post("/cli-exchange", authH.CLIExchange)
		r.Post("/refresh", authH.Refresh)
		r.With(requireAuth).Post("/logout", authH.Logout)
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.APIVersionMiddleware)
		r.Use(middleware.APIRateLimit())
		r.Use(requireAuth)

		r.Get("/whoami", h.Whoami)
		r.Get("/classify", h.Classify)

		r.Route("/profiles", func(r chi.Router) {
			r.Get("/", h.ListProfiles)
			r.Get("/search", h.SearchProfiles)
			r.Get("/export", h.ExportProfiles)
			r.Get("/{id}", h.GetProfile)
			r.With(requireAdmin).Post("/", h.CreateProfile)
			r.With(requireAdmin).Delete("/{id}", h.DeleteProfile)
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
