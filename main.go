package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"
)

var db *pgxpool.Pool
var genderizeBaseURL = "https://api.genderize.io"

// --- Structs ---

type GenderResponse struct {
	Name        string  `json:"name"`
	Gender      string  `json:"gender"`
	Probability float64 `json:"probability"`
	Count       int     `json:"count"`
}

type AgeResponse struct {
	Name  string `json:"name"`
	Age   int    `json:"age"`
	Count int    `json:"count"`
}

type CountryEntry struct {
	CountryID   string  `json:"country_id"`
	Probability float64 `json:"probability"`
}

type NationalizeResponse struct {
	Name    string         `json:"name"`
	Country []CountryEntry `json:"country"`
}

type ErrorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type DataPayload struct {
	Name        string    `json:"name"`
	Gender      string    `json:"gender"`
	Probability float64   `json:"probability"`
	SampleSize  int       `json:"sample_size"`
	IsConfident bool      `json:"is_confident"`
	ProcessedAt time.Time `json:"processed_at"`
}

type ProfilePayload struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Gender             string    `json:"gender"`
	GenderProbability  float64   `json:"gender_probability"`
	SampleSize         int       `json:"sample_size"`
	Age                int       `json:"age"`
	AgeGroup           string    `json:"age_group"`
	CountryID          string    `json:"country_id"`
	CountryProbability float64   `json:"country_probability"`
	CreatedAt          time.Time `json:"created_at"`
}

type SuccessResponse struct {
	Status string      `json:"status"`
	Data   DataPayload `json:"data"`
}

type ProfileResponse struct {
	Status string         `json:"status"`
	Data   ProfilePayload `json:"data"`
}

type ProfileListResponse struct {
	Status string           `json:"status"`
	Count  int              `json:"count"`
	Data   []ProfilePayload `json:"data"`
}

// --- DB ---

func initDB(ctx context.Context) error {
	var err error
	db, err = pgxpool.New(ctx, os.Getenv("DATABASE_URL"))

	if err != nil {
		return err
	}
	resp, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS profiles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			gender TEXT NOT NULL,
			gender_probability NUMERIC NOT NULL,
			sample_size INT NOT NULL,
			age INT NOT NULL,
			age_group TEXT NOT NULL,
			country_id TEXT NOT NULL,
			country_probability NUMERIC NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			
		)
	`)
	if err != nil {
		return err
	}
	
	log.Printf("DB initialized: %v", resp)
	return nil
}

// --- Helpers ---

func writeError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Status: "error", Message: message})
}

func fetchJSON(apiURL string, target interface{}) error {
	resp, err := http.Get(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
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

// --- Handlers ---

func classifyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, "Missing name parameter", http.StatusBadRequest)
		return
	}

	var result GenderResponse
	if err := fetchJSON(genderizeBaseURL+"?name="+url.QueryEscape(name), &result); err != nil {
		log.Printf("Error calling genderize API: %v", err)
		writeError(w, "Failed to call external API", http.StatusBadGateway)
		return
	}

	if result.Gender == "" || result.Count == 0 {
		writeError(w, "No prediction available for the provided name", http.StatusNotFound)
		return
	}

	isConfident := result.Probability >= 0.7 && result.Count >= 100

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(SuccessResponse{
		Status: "success",
		Data: DataPayload{
			Name:        result.Name,
			Gender:      result.Gender,
			Probability: result.Probability,
			SampleSize:  result.Count,
			IsConfident: isConfident,
			ProcessedAt: time.Now().UTC(),
		},
	})
}

func profileCreationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, "Missing or invalid 'name' in request body", http.StatusBadRequest)
		return
	}
	encodedName := url.QueryEscape(body.Name)

	// Idempotency Check: Check if a profile for this name already exists and return it instead of creating a duplicate
	var existingProfile ProfilePayload
	err := db.QueryRow(r.Context(), `SELECT * FROM profiles WHERE name = $1`, body.Name).Scan(
		&existingProfile.ID, 
		&existingProfile.Name,
		&existingProfile.Gender,
		&existingProfile.GenderProbability,
		&existingProfile.SampleSize,
		&existingProfile.Age,
		&existingProfile.AgeGroup,
		&existingProfile.CountryID,
		&existingProfile.CountryProbability,
		&existingProfile.CreatedAt)

	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(ProfileResponse{Status: "success", Data: existingProfile})
		return
	} else if err != nil && err != pgx.ErrNoRows {
		log.Printf("DB query error: %v", err)
		writeError(w, "Failed to check existing profiles", http.StatusInternalServerError)
		return
	}

	// Call all three prediction APIs concurrently
	var genderResult GenderResponse
	var ageResult AgeResponse
	var natResult NationalizeResponse

	g, _ := errgroup.WithContext(r.Context())

	g.Go(func() error {
		return fetchJSON(genderizeBaseURL+"?name="+encodedName, &genderResult)
	})
	g.Go(func() error {
		return fetchJSON("https://api.agify.io?name="+encodedName, &ageResult)
	})
	g.Go(func() error {
		return fetchJSON("https://api.nationalize.io?name="+encodedName, &natResult)
	})

	if err := g.Wait(); err != nil {
		log.Printf("prediction API error: %v", err)
		writeError(w, "Failed to fetch predictions", http.StatusBadGateway)
		return
	}

	if genderResult.Gender == "" || genderResult.Count == 0 {
		writeError(w, "No gender prediction available for the provided name", http.StatusNotFound)
		return
	}
	if ageResult.Age == 0 {
		writeError(w, "No age prediction available for the provided name", http.StatusNotFound)
		return
	}
	if len(natResult.Country) == 0 {
		writeError(w, "No country prediction available for the provided name", http.StatusNotFound)
		return
	}

	// Pick country with highest probability
	topCountry := natResult.Country[0]
	for _, c := range natResult.Country[1:] {
		if c.Probability > topCountry.Probability {
			topCountry = c
		}
	}

	// Store in DB and return the created profile
	var profile ProfilePayload
	err = db.QueryRow(r.Context(), `
		INSERT INTO profiles (name, gender, gender_probability, sample_size, age, age_group, country_id, country_probability)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, gender, gender_probability, sample_size, age, age_group, country_id, country_probability, created_at
	`,
		genderResult.Name,
		genderResult.Gender,
		genderResult.Probability,
		genderResult.Count,
		ageResult.Age,
		computeAgeGroup(ageResult.Age),
		topCountry.CountryID,
		topCountry.Probability,
	).Scan(
		&profile.ID,
		&profile.Name,
		&profile.Gender,
		&profile.GenderProbability,
		&profile.SampleSize,
		&profile.Age,
		&profile.AgeGroup,
		&profile.CountryID,
		&profile.CountryProbability,
		&profile.CreatedAt,
	)
	if err != nil {
		log.Printf("DB insert error: %v", err)
		writeError(w, "Failed to store profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ProfileResponse{Status: "success", Data: profile})
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]string{
		"name":    "Gender Classify API",
		"author":  "Abdulsalam Elhakamy",
		"version": "1.0.0",
		"usage":   "GET /api/classify?name=<name> | POST /api/profile",
	})
}

func profileRetrievalHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, "Missing id parameter", http.StatusBadRequest)
		return
	}

	var profile ProfilePayload
	err := db.QueryRow(r.Context(), `SELECT * FROM profiles WHERE id = $1`, id).Scan(
		&profile.ID,
		&profile.Name,			
		&profile.Gender,
		&profile.GenderProbability,
		&profile.SampleSize,
		&profile.Age,
		&profile.AgeGroup,
		&profile.CountryID,
		&profile.CountryProbability,
		&profile.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		writeError(w, "Profile not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("DB query error: %v", err)
		writeError(w, "Failed to retrieve profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(ProfileResponse{Status: "success", Data: profile})
}

func profileListHandler(w http.ResponseWriter, r *http.Request) {
	// Filter parameters (optional) - supports filtering by gender, country_id, and age_group.
	var conditions []string
	var args []interface{}
	argIdx := 1

	if gender := r.URL.Query().Get("gender"); gender != "" {
		conditions = append(conditions, "LOWER(gender) = LOWER($"+fmt.Sprintf("%d", argIdx)+")")
		args = append(args, gender)
		argIdx++
	}
	if countryID := r.URL.Query().Get("country_id"); countryID != "" {
		conditions = append(conditions, "LOWER(country_id) = LOWER($"+fmt.Sprintf("%d", argIdx)+")")
		args = append(args, countryID)
		argIdx++
	}
	if ageGroup := r.URL.Query().Get("age_group"); ageGroup != "" {
		conditions = append(conditions, "LOWER(age_group) = LOWER($"+fmt.Sprintf("%d", argIdx)+")")
		args = append(args, ageGroup)
	}

	query := `SELECT * FROM profiles`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"

	rows, err := db.Query(r.Context(), query, args...)
	if err != nil {
		log.Printf("DB query error: %v", err)
		writeError(w, "Failed to retrieve profiles", http.StatusInternalServerError)
		return
	}

	defer rows.Close()

	var profiles []ProfilePayload
	for rows.Next() {
		var p ProfilePayload
		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.Gender,
			&p.GenderProbability,
			&p.SampleSize,
			&p.Age,
			&p.AgeGroup,
			&p.CountryID,
			&p.CountryProbability,
			&p.CreatedAt,
		); err != nil {
			log.Printf("DB scan error: %v", err)
			writeError(w, "Failed to retrieve profiles", http.StatusInternalServerError)
			return
		}
		profiles = append(profiles, p)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(ProfileListResponse{Status: "success", Data: profiles, Count: len(profiles)})
}

func profileDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, "Missing id parameter", http.StatusBadRequest)
		return
	}

	cmdTag, err := db.Exec(r.Context(), `DELETE FROM profiles WHERE id = $1`, id)
	if err != nil {
		log.Printf("DB delete error: %v", err)
		writeError(w, "Failed to delete profile", http.StatusInternalServerError)
		return
	}
	if cmdTag.RowsAffected() == 0 {
		writeError(w, "Profile not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusNoContent)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Profile deleted successfully",
	})
}

func main() {
	godotenv.Load()

	if err := initDB(context.Background()); err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	http.HandleFunc("GET /", rootHandler)
	http.HandleFunc("GET /api/classify", classifyHandler)
	http.HandleFunc("POST /api/profiles", profileCreationHandler)
	http.HandleFunc("GET /api/profiles", profileListHandler)
	http.HandleFunc("GET /api/profiles/{id}", profileRetrievalHandler)
	http.HandleFunc("DELETE /api/profiles/{id}", profileDeleteHandler)

	log.Println("Server running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
