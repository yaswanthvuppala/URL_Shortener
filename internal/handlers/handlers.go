package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/yaswanthvuppala/url-shortener/internal/cache"
	"github.com/yaswanthvuppala/url-shortener/internal/models"
)

// URLRepository defines the storage operations required by Handler.
type URLRepository interface {
	CreateURL(ctx context.Context, shortCode, longURL string) error
	GetByShortCode(ctx context.Context, shortCode string) (*models.URLMapping, error)
	IncrementClicks(ctx context.Context, shortCode string) error
	ShortCodeExists(ctx context.Context, shortCode string) (bool, error)
}

// Handler contains all HTTP handler dependencies.
type Handler struct {
	repo     URLRepository
	cache    cache.Cacher
	baseURL  string
	staticFS fs.FS
}

// NewHandler creates a new Handler instance.
func NewHandler(repo URLRepository, c cache.Cacher, baseURL string, staticFS fs.FS) *Handler {
	return &Handler{
		repo:     repo,
		cache:    c,
		baseURL:  strings.TrimRight(baseURL, "/"),
		staticFS: staticFS,
	}
}

// Routes constructs and registers all application routes using Go 1.22+ ServeMux.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	// Web UI
	mux.HandleFunc("GET /{$}", h.ServeRoot)

	// Utility & Documentation
	mux.HandleFunc("GET /health", h.HealthCheck)
	mux.HandleFunc("GET /docs", h.ServeDocs)

	// API Endpoints
	mux.HandleFunc("POST /shorten", h.ShortenURL)
	mux.HandleFunc("GET /analytics/{short_code}", h.GetAnalytics)
	mux.HandleFunc("GET /{short_code}", h.RedirectToLongURL)

	return mux
}

// ServeRoot serves the embedded static index.html file.
func (h *Handler) ServeRoot(w http.ResponseWriter, r *http.Request) {
	if h.staticFS == nil {
		RespondError(w, http.StatusNotFound, "Static assets not available")
		return
	}

	data, err := fs.ReadFile(h.staticFS, "index.html")
	if err != nil {
		log.Printf("Failed reading embedded index.html: %v", err)
		RespondError(w, http.StatusNotFound, "Frontend index.html not found")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// HealthCheck returns a basic JSON status for container health monitors.
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

// ServeDocs provides an overview of available endpoints for developers.
func (h *Handler) ServeDocs(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]any{
		"title":       "URL Shortener API",
		"description": "Standard Go URL Shortener Service (CGO-free)",
		"endpoints": []map[string]string{
			{"method": "GET", "path": "/", "description": "Web Interface"},
			{"method": "POST", "path": "/shorten", "description": "Create shortened URL"},
			{"method": "GET", "path": "/{short_code}", "description": "307 redirect to target URL"},
			{"method": "GET", "path": "/analytics/{short_code}", "description": "Get analytics and click counts"},
			{"method": "GET", "path": "/health", "description": "Health status probe"},
		},
	})
}

// ShortenURL handles creating a shortened URL.
func (h *Handler) ShortenURL(w http.ResponseWriter, r *http.Request) {
	var req models.ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	validatedURL, err := ValidateURL(req.LongURL)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	var shortCode string
	var created bool

	// Retry loop for collisions (up to 10 attempts)
	for attempt := 0; attempt < 10; attempt++ {
		code, genErr := GenerateShortCode(6)
		if genErr != nil {
			log.Printf("Error generating short code: %v", genErr)
			continue
		}

		exists, existErr := h.repo.ShortCodeExists(ctx, code)
		if existErr != nil {
			log.Printf("Error checking short code existence: %v", existErr)
			continue
		}
		if exists {
			continue // collision, retry
		}

		if insErr := h.repo.CreateURL(ctx, code, validatedURL.String()); insErr != nil {
			log.Printf("Insert conflict for code %s: %v", code, insErr)
			continue
		}

		shortCode = code
		created = true
		break
	}

	if !created {
		RespondError(w, http.StatusInternalServerError, "Could not generate unique short code")
		return
	}

	// Write-through cache with 24-hour TTL (86400s)
	if h.cache != nil {
		h.cache.Set(ctx, shortCode, validatedURL.String(), 24*time.Hour)
	}

	shortURL := fmt.Sprintf("%s/%s", h.baseURL, shortCode)
	RespondJSON(w, http.StatusCreated, models.ShortenResponse{
		ShortURL: shortURL,
	})
}

// RedirectToLongURL handles resolving the short code and redirecting to the target URL.
func (h *Handler) RedirectToLongURL(w http.ResponseWriter, r *http.Request) {
	shortCode := strings.TrimSpace(r.PathValue("short_code"))
	if shortCode == "" {
		RespondError(w, http.StatusNotFound, "Short URL not found")
		return
	}

	ctx := r.Context()

	// 1. Try cache lookup first (Cache Hit)
	if h.cache != nil {
		if cachedURL, hit := h.cache.Get(ctx, shortCode); hit && cachedURL != "" {
			// Increment clicks in DB asynchronously or directly
			_ = h.repo.IncrementClicks(ctx, shortCode)
			http.Redirect(w, r, cachedURL, http.StatusTemporaryRedirect)
			return
		}
	}

	// 2. Cache Miss: Query Database
	mapping, err := h.repo.GetByShortCode(ctx, shortCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			RespondError(w, http.StatusNotFound, "Short URL not found")
			return
		}
		log.Printf("Database lookup error for %s: %v", shortCode, err)
		RespondError(w, http.StatusInternalServerError, "Internal database error")
		return
	}

	// Populate cache with 24-hour TTL
	if h.cache != nil {
		h.cache.Set(ctx, shortCode, mapping.LongURL, 24*time.Hour)
	}

	// Increment clicks
	_ = h.repo.IncrementClicks(ctx, shortCode)

	// 307 Temporary Redirect
	http.Redirect(w, r, mapping.LongURL, http.StatusTemporaryRedirect)
}

// GetAnalytics returns click counts and creation timestamp for a short URL.
func (h *Handler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	shortCode := strings.TrimSpace(r.PathValue("short_code"))
	if shortCode == "" {
		RespondError(w, http.StatusNotFound, "Short URL not found")
		return
	}

	ctx := r.Context()
	mapping, err := h.repo.GetByShortCode(ctx, shortCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			RespondError(w, http.StatusNotFound, "Short URL not found")
			return
		}
		log.Printf("Analytics DB error for %s: %v", shortCode, err)
		RespondError(w, http.StatusInternalServerError, "Internal database error")
		return
	}

	RespondJSON(w, http.StatusOK, models.AnalyticsResponse{
		ShortCode: mapping.ShortCode,
		LongURL:   mapping.LongURL,
		Clicks:    mapping.Clicks,
		CreatedAt: mapping.CreatedAt,
	})
}
