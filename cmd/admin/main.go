// cmd/admin promotes a GitHub user to the admin role.
//
// Usage:
//
//	go run ./cmd/admin <github-username>
//
// The user must have logged in at least once (so their record exists in the DB).
// Run this after your first GitHub OAuth login to grant yourself admin access.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"hng14-s0-gender-classify/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/admin <github-username>")
		os.Exit(1)
	}
	username := os.Args[1]

	godotenv.Load()
	cfg := config.Load()
	ctx := context.Background()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("DB connect failed: %v", err)
	}
	defer db.Close()

	tag, err := db.Exec(ctx,
		`UPDATE users SET role = 'admin' WHERE username = $1`,
		username,
	)
	if err != nil {
		log.Fatalf("Update failed: %v", err)
	}
	if tag.RowsAffected() == 0 {
		log.Fatalf("User %q not found — log in via the portal first, then re-run this.", username)
	}

	fmt.Printf("✓ %s is now an admin. Log out and back in for the new role to take effect.\n", username)
}
