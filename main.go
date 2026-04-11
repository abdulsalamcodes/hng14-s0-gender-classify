package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

var genderizeBaseURL = "https://api.genderize.io"

type GenderResponse struct {
	Name        string  `json:"name"`
	Gender      string  `json:"gender"`
	Probability float64 `json:"probability"`
	Count       int     `json:"count"`
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

type SuccessResponse struct {
	Status string      `json:"status"`
	Data   DataPayload `json:"data"`
}

func writeError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Status: "error", Message: message})
}

func classifyHandler(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests.
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Input validation
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, "Missing name parameter", http.StatusBadRequest)
		return
	}

	// Call the external API.
	apiURL := genderizeBaseURL + "?name=" + url.QueryEscape(name)
	resp, err := http.Get(apiURL)
	if err != nil {
		log.Printf("Error calling external API: %v", err)
		writeError(w, "Failed to call external API", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, "External API returned an error", http.StatusBadGateway)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	var result GenderResponse
	if err := json.Unmarshal(body, &result); err != nil {
		writeError(w, "Failed to parse response", http.StatusInternalServerError)
		return
	}

	// Edge case: API couldn't determine a gender for this name.
	if result.Gender == "" || result.Count == 0 {
		writeError(w, "No prediction available for the provided name", http.StatusNotFound)
		return
	}

	isConfident := result.Probability >= 0.7 && result.Count >= 100
	processedAt := time.Now().UTC()

	response := SuccessResponse{
		Status: "success",
		Data: DataPayload{
			Name:        result.Name,
			Gender:      result.Gender,
			Probability: result.Probability,
			SampleSize:  result.Count,
			IsConfident: isConfident,
			ProcessedAt: processedAt,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	json.NewEncoder(w).Encode(response)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]string{
		"name":    "Gender Classify API",
		"author":  "Abdulsalam Elhakamy",
		"version": "1.0.0",
		"usage":   "GET /api/classify?name=<name>",
	})
}

func main() {
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/api/classify", classifyHandler)
	log.Println("Server running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
