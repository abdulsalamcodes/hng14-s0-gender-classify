package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"hng14-s0-gender-classify/internal/config"
	"hng14-s0-gender-classify/internal/models"
	"hng14-s0-gender-classify/internal/repository"
	"hng14-s0-gender-classify/internal/services"
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

	svc := services.New(repo, nil)

	data, err := os.ReadFile("internal/data/seed_profiles.json")
	if err != nil {
		log.Fatalf("Failed to read seed data: %v", err)
	}

	var seedData models.SeedData
	if err := json.Unmarshal(data, &seedData); err != nil {
		log.Fatalf("Failed to parse seed data: %v", err)
	}

	log.Printf("Seeding %d profiles...", len(seedData.Profiles))

	for i, p := range seedData.Profiles {
		if err := svc.SeedProfile(ctx, &p); err != nil {
			log.Printf("Failed to seed profile %s: %v", p.Name, err)
			continue
		}
		if (i+1)%100 == 0 {
			log.Printf("Seeded %d profiles...", i+1)
		}
	}

	log.Println("Seeding completed!")
}
