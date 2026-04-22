package models

import "time"

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
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Gender              string    `json:"gender"`
	GenderProbability   float64   `json:"gender_probability"`
	SampleSize          int       `json:"sample_size"`
	Age                 int       `json:"age"`
	AgeGroup            string    `json:"age_group"`
	CountryID           string    `json:"country_id"`
	CountryName         string    `json:"country_name"`
	CountryProbability  float64   `json:"country_probability"`
	CreatedAt           time.Time `json:"created_at"`
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
	Page   int              `json:"page"`
	Limit  int              `json:"limit"`
	Total  int              `json:"total"`
	Data   []ProfilePayload `json:"data"`
}

type RootResponse struct {
	Name    string `json:"name"`
	Author  string `json:"author"`
	Version string `json:"version"`
	Usage   string `json:"usage"`
}

type SeedProfile struct {
	Name               string  `json:"name"`
	Gender             string  `json:"gender"`
	GenderProbability  float64 `json:"gender_probability"`
	Age                int     `json:"age"`
	AgeGroup           string  `json:"age_group"`
	CountryID          string  `json:"country_id"`
	CountryName        string  `json:"country_name"`
	CountryProbability float64 `json:"country_probability"`
}

type SeedData struct {
	Profiles []SeedProfile `json:"profiles"`
}
