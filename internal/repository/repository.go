package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"hng14-s0-gender-classify/internal/models"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) InitSchema(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS profiles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(255) NOT NULL UNIQUE,
			gender VARCHAR(10) NOT NULL,
			gender_probability FLOAT NOT NULL,
			sample_size INT NOT NULL,
			age INT NOT NULL,
			age_group VARCHAR(20) NOT NULL,
			country_id VARCHAR(2) NOT NULL,
			country_name VARCHAR(255) NOT NULL,
			country_probability FLOAT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func (r *Repository) GetProfileByName(ctx context.Context, name string) (*models.ProfilePayload, error) {
	var profile models.ProfilePayload
	err := r.db.QueryRow(ctx, `SELECT * FROM profiles WHERE name = $1`, name).Scan(
		&profile.ID,
		&profile.Name,
		&profile.Gender,
		&profile.GenderProbability,
		&profile.SampleSize,
		&profile.Age,
		&profile.AgeGroup,
		&profile.CountryID,
		&profile.CountryName,
		&profile.CountryProbability,
		&profile.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *Repository) GetProfileByID(ctx context.Context, id string) (*models.ProfilePayload, error) {
	var profile models.ProfilePayload
	err := r.db.QueryRow(ctx, `SELECT * FROM profiles WHERE id = $1`, id).Scan(
		&profile.ID,
		&profile.Name,
		&profile.Gender,
		&profile.GenderProbability,
		&profile.SampleSize,
		&profile.Age,
		&profile.AgeGroup,
		&profile.CountryID,
		&profile.CountryName,
		&profile.CountryProbability,
		&profile.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

type ProfileFilter struct {
	Gender              string
	AgeGroup            string
	CountryID           string
	MinAge              *int
	MaxAge              *int
	MinGenderProbability *float64
	MinCountryProbability *float64
	SortBy              string
	Order               string
	Page                int
	Limit               int
}

type PaginatedResult struct {
	Data  []models.ProfilePayload
	Total int
}

func (r *Repository) ListProfiles(ctx context.Context, filter ProfileFilter) (*PaginatedResult, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if filter.Gender != "" {
		conditions = append(conditions, fmt.Sprintf("LOWER(gender) = LOWER($%d)", argIdx))
		args = append(args, filter.Gender)
		argIdx++
	}
	if filter.CountryID != "" {
		conditions = append(conditions, fmt.Sprintf("LOWER(country_id) = LOWER($%d)", argIdx))
		args = append(args, filter.CountryID)
		argIdx++
	}
	if filter.AgeGroup != "" {
		conditions = append(conditions, fmt.Sprintf("LOWER(age_group) = LOWER($%d)", argIdx))
		args = append(args, filter.AgeGroup)
		argIdx++
	}
	if filter.MinAge != nil {
		conditions = append(conditions, fmt.Sprintf("age >= $%d", argIdx))
		args = append(args, *filter.MinAge)
		argIdx++
	}
	if filter.MaxAge != nil {
		conditions = append(conditions, fmt.Sprintf("age <= $%d", argIdx))
		args = append(args, *filter.MaxAge)
		argIdx++
	}
	if filter.MinGenderProbability != nil {
		conditions = append(conditions, fmt.Sprintf("gender_probability >= $%d", argIdx))
		args = append(args, *filter.MinGenderProbability)
		argIdx++
	}
	if filter.MinCountryProbability != nil {
		conditions = append(conditions, fmt.Sprintf("country_probability >= $%d", argIdx))
		args = append(args, *filter.MinCountryProbability)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + conditions[0]
		for i := 1; i < len(conditions); i++ {
			whereClause += " AND " + conditions[i]
		}
	}

	countQuery := "SELECT COUNT(*) FROM profiles" + whereClause
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	sortColumn := "created_at"
	switch filter.SortBy {
	case "age":
		sortColumn = "age"
	case "created_at":
		sortColumn = "created_at"
	case "gender_probability":
		sortColumn = "gender_probability"
	}

	sortOrder := "DESC"
	if filter.Order == "asc" {
		sortOrder = "ASC"
	}

	offset := (filter.Page - 1) * filter.Limit
	query := fmt.Sprintf(`SELECT id, name, gender, gender_probability, sample_size, age, age_group, country_id, country_name, country_probability, created_at FROM profiles%s ORDER BY %s %s LIMIT %d OFFSET %d`, whereClause, sortColumn, sortOrder, filter.Limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []models.ProfilePayload
	for rows.Next() {
		var p models.ProfilePayload
		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.Gender,
			&p.GenderProbability,
			&p.SampleSize,
			&p.Age,
			&p.AgeGroup,
			&p.CountryID,
			&p.CountryName,
			&p.CountryProbability,
			&p.CreatedAt,
		); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return &PaginatedResult{Data: profiles, Total: total}, nil
}

func (r *Repository) CreateProfile(ctx context.Context, p *models.ProfilePayload) (*models.ProfilePayload, error) {
	err := r.db.QueryRow(ctx, `
		INSERT INTO profiles (name, gender, gender_probability, sample_size, age, age_group, country_id, country_name, country_probability)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, name, gender, gender_probability, sample_size, age, age_group, country_id, country_name, country_probability, created_at
	`,
		p.Name,
		p.Gender,
		p.GenderProbability,
		p.SampleSize,
		p.Age,
		p.AgeGroup,
		p.CountryID,
		p.CountryName,
		p.CountryProbability,
	).Scan(
		&p.ID,
		&p.Name,
		&p.Gender,
		&p.GenderProbability,
		&p.SampleSize,
		&p.Age,
		&p.AgeGroup,
		&p.CountryID,
		&p.CountryName,
		&p.CountryProbability,
		&p.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Repository) DeleteProfile(ctx context.Context, id string) (bool, error) {
	cmdTag, err := r.db.Exec(ctx, `DELETE FROM profiles WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return cmdTag.RowsAffected() > 0, nil
}

func (r *Repository) SeedProfile(ctx context.Context, p *models.SeedProfile) error {
	ageGroup := computeAgeGroup(p.Age)
	_, err := r.db.Exec(ctx, `
		INSERT INTO profiles (name, gender, gender_probability, sample_size, age, age_group, country_id, country_name, country_probability)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (name) DO NOTHING
	`,
		p.Name,
		p.Gender,
		p.GenderProbability,
		int(p.GenderProbability*100),
		p.Age,
		ageGroup,
		p.CountryID,
		p.CountryName,
		p.CountryProbability,
	)
	return err
}

func (r *Repository) Close() {
	r.db.Close()
}

func computeAgeGroup(age int) string {
	switch {
	case age <= 12:
		return "child"
	case age <= 19:
		return "teenager"
	case age <= 59:
		return "adult"
	default:
		return "senior"
	}
}

func ComputeAgeGroup(age int) string {
	return computeAgeGroup(age)
}
