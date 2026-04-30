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

	// ── Database ──────────────────────────────────────────────────────────────
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	repo := repository.New(db)
	if err := repo.InitSchema(ctx); err != nil {
		log.Fatalf("Failed to init schema: %v", err)
	}

	// ── Services ──────────────────────────────────────────────────────────────
	// Token service: handles JWT signing/validation for both access and refresh tokens.
	tokenSvc := services.NewTokenService(cfg.JWTSecret, cfg.JWTRefreshSecret)

	// Auth service: handles the GitHub OAuth flow, user upsert, and token lifecycle.
	authSvc := services.NewAuthService(
		repo, tokenSvc,
		cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.GitHubRedirectURL,
	)

	// Profile service: the Stage 1/2 business logic (create, list, search profiles).
	apiClient := api.NewClient(cfg.GenderizeURL, cfg.AgifyURL, cfg.NationalizeURL)
	profileSvc := services.New(repo, apiClient)

	// ── Handlers ──────────────────────────────────────────────────────────────
	authH := handlers.NewAuthHandler(authSvc, tokenSvc, cfg.FrontendURL)
	h := handlers.New(profileSvc)

	// ── Shared middleware factories ────────────────────────────────────────────
	// We build these once and reuse them — each call to e.g. middleware.RequireAuth(...)
	// returns a new middleware function, so we capture them in variables.
	requireAuth := middleware.RequireAuth(tokenSvc, repo)
	requireAdmin := middleware.RequireRole("admin")

	// ── Router ────────────────────────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware (applied to every request):
	// 1. LoggingMiddleware — logs method/path/status/duration. Outermost so it
	//    captures the full request lifetime including auth overhead.
	// 2. Recoverer — catches panics and returns 500 instead of crashing.
	r.Use(middleware.LoggingMiddleware)
	r.Use(chimiddleware.Recoverer)

	// Root health-check (no auth, no versioning required).
	r.Get("/", h.Root)

	// ── Auth routes (/auth/*) ─────────────────────────────────────────────────
	// These routes are public (no RequireAuth) but rate-limited to 10 req/min/IP
	// to prevent OAuth flow abuse.
	r.Route("/auth", func(r chi.Router) {
		r.Use(middleware.AuthRateLimit())

		// GET /auth/github — starts the OAuth flow (redirects to GitHub)
		r.Get("/github", authH.GitHubLogin)
		// GET /auth/github/callback — GitHub redirects here after user approves
		r.Get("/github/callback", authH.GitHubCallback)
		// POST /auth/refresh — rotates tokens; accepts JSON body or cookie
		r.Post("/refresh", authH.Refresh)
		// POST /auth/logout — invalidates all refresh tokens for the user
		// RequireAuth here so we know WHO is logging out.
		r.With(requireAuth).Post("/logout", authH.Logout)
	})

	// ── API routes (/api/*) ───────────────────────────────────────────────────
	// All /api/* routes:
	//   1. Must carry X-API-Version: 1 header (APIVersionMiddleware)
	//   2. Must be authenticated (requireAuth)
	//   3. Are rate-limited to 60 req/min per user (APIRateLimit)
	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.APIVersionMiddleware)
		r.Use(middleware.APIRateLimit())
		r.Use(requireAuth)

		// GET /api/whoami — returns the currently authenticated user's info.
		r.Get("/whoami", h.Whoami)

		// GET /api/classify — legacy single-name classification (no DB write).
		r.Get("/classify", h.Classify)

		r.Route("/profiles", func(r chi.Router) {
			// Read-only endpoints — accessible by both admin and analyst.
			r.Get("/", h.ListProfiles)
			r.Get("/search", h.SearchProfiles)
			r.Get("/export", h.ExportProfiles) // CSV download
			r.Get("/{id}", h.GetProfile)

			// Write endpoints — admin only.
			// r.With(...) adds middleware for just this specific route,
			// without affecting the other routes in this group.
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
