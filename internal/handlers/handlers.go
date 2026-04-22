package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"hng14-s0-gender-classify/internal/models"
	"hng14-s0-gender-classify/internal/repository"
	"hng14-s0-gender-classify/internal/services"
)

type Handler struct {
	service *services.Service
}

func New(service *services.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Root(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(models.RootResponse{
		Name:    "Gender Classify API",
		Author:  "Abdulsalam Elhakamy",
		Version: "1.0.0",
		Usage:   "GET /api/classify?name=<name> | POST /api/profile",
	})
}

func (h *Handler) Classify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, "Missing name parameter", http.StatusBadRequest)
		return
	}

	result, err := h.service.ClassifyName(r.Context(), name)
	if err != nil {
		log.Printf("Error classifying name: %v", err)
		writeError(w, "Failed to classify name", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(models.SuccessResponse{
		Status: "success",
		Data:   *result,
	})
}

func (h *Handler) CreateProfile(w http.ResponseWriter, r *http.Request) {
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

	profile, err := h.service.CreateProfile(r.Context(), body.Name)
	if err != nil {
		log.Printf("Error creating profile: %v", err)
		writeError(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(models.ProfileResponse{Status: "success", Data: *profile})
}

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Missing id parameter", http.StatusBadRequest)
		return
	}

	profile, err := h.service.GetProfile(r.Context(), id)
	if err != nil {
		log.Printf("Error getting profile: %v", err)
		writeError(w, "Failed to retrieve profile", http.StatusInternalServerError)
		return
	}
	if profile == nil {
		writeError(w, "Profile not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(models.ProfileResponse{Status: "success", Data: *profile})
}

func (h *Handler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	filter := repository.ProfileFilter{
		Gender:    r.URL.Query().Get("gender"),
		AgeGroup:  r.URL.Query().Get("age_group"),
		CountryID: r.URL.Query().Get("country_id"),
	}

	if minAge := r.URL.Query().Get("min_age"); minAge != "" {
		var val int
		if _, err := fmt.Sscanf(minAge, "%d", &val); err == nil {
			filter.MinAge = &val
		}
	}
	if maxAge := r.URL.Query().Get("max_age"); maxAge != "" {
		var val int
		if _, err := fmt.Sscanf(maxAge, "%d", &val); err == nil {
			filter.MaxAge = &val
		}
	}
	if minGenderProb := r.URL.Query().Get("min_gender_probability"); minGenderProb != "" {
		var val float64
		if _, err := fmt.Sscanf(minGenderProb, "%f", &val); err == nil {
			filter.MinGenderProbability = &val
		}
	}
	if minCountryProb := r.URL.Query().Get("min_country_probability"); minCountryProb != "" {
		var val float64
		if _, err := fmt.Sscanf(minCountryProb, "%f", &val); err == nil {
			filter.MinCountryProbability = &val
		}
	}

	filter.SortBy = r.URL.Query().Get("sort_by")
	filter.Order = r.URL.Query().Get("order")

	page := 1
	limit := 10
	if p := r.URL.Query().Get("page"); p != "" {
		var parsed int
		if _, err := fmt.Sscanf(p, "%d", &parsed); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		var parsed int
		if _, err := fmt.Sscanf(l, "%d", &parsed); err == nil && parsed > 0 {
			if parsed > 50 {
				parsed = 50
			}
			limit = parsed
		}
	}
	filter.Page = page
	filter.Limit = limit

	result, err := h.service.ListProfiles(r.Context(), filter)
	if err != nil {
		log.Printf("Error listing profiles: %v", err)
		writeError(w, "Failed to retrieve profiles", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(models.ProfileListResponse{
		Status: "success",
		Page:   page,
		Limit:  limit,
		Total:  result.Total,
		Data:   result.Data,
	})
}

func (h *Handler) SearchProfiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, "Missing 'q' parameter", http.StatusBadRequest)
		return
	}

	filter := services.ParseSearchQuery(q)

	page := 1
	limit := 10
	if p := r.URL.Query().Get("page"); p != "" {
		var parsed int
		if _, err := fmt.Sscanf(p, "%d", &parsed); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		var parsed int
		if _, err := fmt.Sscanf(l, "%d", &parsed); err == nil && parsed > 0 {
			if parsed > 50 {
				parsed = 50
			}
			limit = parsed
		}
	}
	filter.Page = page
	filter.Limit = limit

	result, err := h.service.ListProfiles(r.Context(), filter)
	if err != nil {
		log.Printf("Error searching profiles: %v", err)
		writeError(w, "Failed to search profiles", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(models.ProfileListResponse{
		Status: "success",
		Page:   page,
		Limit:  limit,
		Total:  result.Total,
		Data:   result.Data,
	})
}

func (h *Handler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Missing id parameter", http.StatusBadRequest)
		return
	}

	deleted, err := h.service.DeleteProfile(r.Context(), id)
	if err != nil {
		log.Printf("Error deleting profile: %v", err)
		writeError(w, "Failed to delete profile", http.StatusInternalServerError)
		return
	}
	if !deleted {
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

func writeError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(models.ErrorResponse{Status: "error", Message: message})
}
