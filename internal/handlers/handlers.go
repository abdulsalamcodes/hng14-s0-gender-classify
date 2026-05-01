package handlers

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"hng14-s0-gender-classify/internal/middleware"
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

	// Validate filter values
	if filter.MinAge != nil && *filter.MinAge < 0 {
		writeError(w, "Invalid query parameters", http.StatusBadRequest)
		return
	}
	if filter.MaxAge != nil && *filter.MaxAge < 0 {
		writeError(w, "Invalid query parameters", http.StatusBadRequest)
		return
	}
	if filter.MinGenderProbability != nil && (*filter.MinGenderProbability < 0 || *filter.MinGenderProbability > 1) {
		writeError(w, "Invalid query parameters", http.StatusBadRequest)
		return
	}
	if filter.MinCountryProbability != nil && (*filter.MinCountryProbability < 0 || *filter.MinCountryProbability > 1) {
		writeError(w, "Invalid query parameters", http.StatusBadRequest)
		return
	}

	result, err := h.service.ListProfiles(r.Context(), filter)
	if err != nil {
		log.Printf("Error listing profiles: %v", err)
		writeError(w, "Failed to retrieve profiles", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(buildListResponse("profiles", r, page, limit, result.Total, result.Data))
}

func (h *Handler) SearchProfiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, "Missing 'q' parameter", http.StatusBadRequest)
		return
	}

	filter, err := services.ParseSearchQuery(q)
	if err != nil {
		if errors.Is(err, services.ErrUnableToParseQuery) {
			writeError(w, "Invalid query parameters", http.StatusBadRequest)
			return
		}
		writeError(w, "Failed to parse query", http.StatusBadRequest)
		return
	}

	page := 1
	limit := 10
	if p := r.URL.Query().Get("page"); p != "" {
		var parsed int
		if _, err := fmt.Sscanf(p, "%d", &parsed); err != nil {
			writeError(w, "Invalid query parameters", http.StatusBadRequest)
			return
		}
		if parsed <= 0 {
			writeError(w, "Invalid query parameters", http.StatusBadRequest)
			return
		}
		page = parsed
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		var parsed int
		if _, err := fmt.Sscanf(l, "%d", &parsed); err != nil {
			writeError(w, "Invalid query parameters", http.StatusBadRequest)
			return
		}
		if parsed <= 0 {
			writeError(w, "Invalid query parameters", http.StatusBadRequest)
			return
		}
		if parsed > 50 {
			writeError(w, "Invalid query parameters", http.StatusBadRequest)
			return
		}
		limit = parsed
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
	json.NewEncoder(w).Encode(buildListResponse("profiles/search", r, page, limit, result.Total, result.Data))
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

// ExportProfiles streams all matching profiles as a CSV file download.
// GET /api/profiles/export?format=csv&gender=...&country_id=...&sort_by=...
//
// We use encoding/csv instead of fmt.Fprintf to correctly handle values
// that contain commas or quotes (e.g. country names like "Korea, South").
func (h *Handler) ExportProfiles(w http.ResponseWriter, r *http.Request) {
	filter := repository.ProfileFilter{
		Gender:    r.URL.Query().Get("gender"),
		AgeGroup:  r.URL.Query().Get("age_group"),
		CountryID: r.URL.Query().Get("country_id"),
		SortBy:    r.URL.Query().Get("sort_by"),
		Order:     r.URL.Query().Get("order"),
		// Fetch all rows — no pagination for exports.
		Page:  1,
		Limit: 10000,
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

	result, err := h.service.ListProfiles(r.Context(), filter)
	if err != nil {
		log.Printf("Error exporting profiles: %v", err)
		writeError(w, "Failed to export profiles", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("profiles_%s.csv", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	// csv.NewWriter handles quoting, escaping, and line endings for us.
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Header row — must match the spec's column order exactly.
	cw.Write([]string{"id", "name", "gender", "gender_probability", "age", "age_group", "country_id", "country_name", "country_probability", "created_at"})

	for _, p := range result.Data {
		cw.Write([]string{
			p.ID,
			p.Name,
			p.Gender,
			fmt.Sprintf("%.4f", p.GenderProbability),
			fmt.Sprintf("%d", p.Age),
			p.AgeGroup,
			p.CountryID,
			p.CountryName,
			fmt.Sprintf("%.4f", p.CountryProbability),
			p.CreatedAt.Format(time.RFC3339),
		})
	}
}

// Whoami returns the full profile of the currently authenticated user.
// GET /api/whoami
// Auth: RequireAuth (any role)
func (h *Handler) Whoami(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.service.GetUser(r.Context(), claims.UserID)
	if err != nil || user == nil {
		writeError(w, "user not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"id":            user.ID,
			"username":      user.Username,
			"email":         user.Email,
			"avatar_url":    user.AvatarURL,
			"role":          user.Role,
			"last_login_at": user.LastLoginAt,
		},
	})
}

// buildListResponse constructs the paginated list response including
// total_pages and HATEOAS-style links (self / next / prev).
//
// Why compute links server-side? The client doesn't need to know the URL
// structure — it can just follow links.next to paginate. This also means
// changing the URL structure in future won't break clients.
func buildListResponse(basePath string, r *http.Request, page, limit, total int, data []models.ProfilePayload) models.ProfileListResponse {
	totalPages := (total + limit - 1) / limit // integer ceiling division
	if totalPages == 0 {
		totalPages = 1
	}

	// Reconstruct the base URL, preserving all query params except page.
	baseURL := "/api/" + basePath

	makeLink := func(p int) *string {
		q := r.URL.Query()
		q.Set("page", fmt.Sprintf("%d", p))
		q.Set("limit", fmt.Sprintf("%d", limit))
		s := baseURL + "?" + q.Encode()
		return &s
	}

	self := makeLink(page)
	var next, prev *string
	if page < totalPages {
		next = makeLink(page + 1)
	}
	if page > 1 {
		prev = makeLink(page - 1)
	}

	return models.ProfileListResponse{
		Status:     "success",
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		Links: map[string]*string{
			"self": self,
			"next": next,
			"prev": prev,
		},
		Data: data,
	}
}

func writeError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(models.ErrorResponse{Status: "error", Message: message})
}
