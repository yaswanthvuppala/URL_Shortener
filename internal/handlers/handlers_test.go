package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/yaswanthvuppala/url-shortener/internal/models"
)

// inMemoryRepo provides a thread-safe in-memory URLRepository for testing.
type inMemoryRepo struct {
	mu           sync.Mutex
	data         map[string]*models.URLMapping
	idCounter    int64
	failInserts  bool
	alwaysExists bool
}

func newInMemoryRepo() *inMemoryRepo {
	return &inMemoryRepo{
		data: make(map[string]*models.URLMapping),
	}
}

func (r *inMemoryRepo) CreateURL(ctx context.Context, shortCode, longURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.failInserts {
		return errors.New("simulated insert error")
	}

	if _, exists := r.data[shortCode]; exists {
		return errors.New("unique constraint violation")
	}

	r.idCounter++
	r.data[shortCode] = &models.URLMapping{
		ID:        r.idCounter,
		ShortCode: shortCode,
		LongURL:   longURL,
		Clicks:    0,
		CreatedAt: time.Now().UTC(),
	}
	return nil
}

func (r *inMemoryRepo) GetByShortCode(ctx context.Context, shortCode string) (*models.URLMapping, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mapping, exists := r.data[shortCode]
	if !exists {
		return nil, sql.ErrNoRows
	}
	// Return a copy
	cp := *mapping
	return &cp, nil
}

func (r *inMemoryRepo) IncrementClicks(ctx context.Context, shortCode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mapping, exists := r.data[shortCode]
	if !exists {
		return sql.ErrNoRows
	}
	mapping.Clicks++
	return nil
}

func (r *inMemoryRepo) ShortCodeExists(ctx context.Context, shortCode string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.alwaysExists {
		return true, nil
	}

	_, exists := r.data[shortCode]
	return exists, nil
}

// inMemoryCache provides a simple in-memory cache for testing.
type inMemoryCache struct {
	mu   sync.Mutex
	data map[string]string
}

func newInMemoryCache() *inMemoryCache {
	return &inMemoryCache{data: make(map[string]string)}
}

func (c *inMemoryCache) Get(ctx context.Context, key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := c.data[key]
	return val, ok
}

func (c *inMemoryCache) Set(ctx context.Context, key string, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

func (c *inMemoryCache) Close() error {
	return nil
}

func setupTestServer() (http.Handler, *inMemoryRepo, *inMemoryCache) {
	repo := newInMemoryRepo()
	cache := newInMemoryCache()
	mockFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<html><body>URL Shortener Test</body></html>"),
		},
	}

	h := NewHandler(repo, cache, "http://testserver:8000", mockFS)
	mux := h.Routes()
	handler := Chain(mux, CORS, Logger, Recoverer)
	return handler, repo, cache
}

func TestServeRoot(t *testing.T) {
	handler, _, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "URL Shortener Test") {
		t.Errorf("expected HTML body to contain 'URL Shortener Test'")
	}
}

func TestServeDocsAndHealth(t *testing.T) {
	handler, _, _ := setupTestServer()

	// Docs
	reqDocs := httptest.NewRequest(http.MethodGet, "/docs", nil)
	recDocs := httptest.NewRecorder()
	handler.ServeHTTP(recDocs, reqDocs)
	if recDocs.Code != http.StatusOK {
		t.Errorf("expected status 200 for /docs, got %d", recDocs.Code)
	}

	// Health
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Errorf("expected status 200 for /health, got %d", recHealth.Code)
	}
}

func TestShortenURL(t *testing.T) {
	handler, repo, cache := setupTestServer()

	// 1. Valid URL
	body := `{"long_url": "https://go.dev/doc"}`
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.ShortenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse shorten response: %v", err)
	}

	if !strings.HasPrefix(resp.ShortURL, "http://testserver:8000/") {
		t.Errorf("unexpected short URL format: %s", resp.ShortURL)
	}

	shortCode := strings.TrimPrefix(resp.ShortURL, "http://testserver:8000/")
	if len(shortCode) != 6 {
		t.Errorf("expected code length 6, got %d (%s)", len(shortCode), shortCode)
	}

	// Verify DB entry
	mapping, err := repo.GetByShortCode(context.Background(), shortCode)
	if err != nil {
		t.Fatalf("code not found in db: %v", err)
	}
	if mapping.LongURL != "https://go.dev/doc" {
		t.Errorf("expected long URL https://go.dev/doc, got %s", mapping.LongURL)
	}

	// Verify Cache entry
	cachedURL, hit := cache.Get(context.Background(), shortCode)
	if !hit || cachedURL != "https://go.dev/doc" {
		t.Errorf("expected cache hit with https://go.dev/doc, got %s (hit=%v)", cachedURL, hit)
	}

	// 2. Invalid URL
	bodyInvalid := `{"long_url": "ftp://not-supported.com"}`
	reqInvalid := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(bodyInvalid))
	recInvalid := httptest.NewRecorder()
	handler.ServeHTTP(recInvalid, reqInvalid)
	if recInvalid.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unsupported scheme, got %d", recInvalid.Code)
	}

	// 3. Malformed JSON
	reqBadJSON := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(`{malformed`))
	recBadJSON := httptest.NewRecorder()
	handler.ServeHTTP(recBadJSON, reqBadJSON)
	if recBadJSON.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed json, got %d", recBadJSON.Code)
	}
}

func TestShortenCollisionRetryFailure(t *testing.T) {
	repo := newInMemoryRepo()
	repo.alwaysExists = true // Force collisions on every attempt
	h := NewHandler(repo, nil, "http://localhost:8000", nil)

	body := `{"long_url": "https://example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	h.ShortenURL(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when all 10 collision retries fail, got %d", rec.Code)
	}
}

func TestRedirectFlow(t *testing.T) {
	handler, repo, cache := setupTestServer()
	ctx := context.Background()

	code := "red001"
	targetURL := "https://example.org/destination"
	if err := repo.CreateURL(ctx, code, targetURL); err != nil {
		t.Fatalf("failed setup: %v", err)
	}

	// First request: Cache Miss -> Populates cache, increments click, redirects 307
	req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected status 307, got %d", rec.Code)
	}
	if rec.Header().Get("Location") != targetURL {
		t.Errorf("expected Location %s, got %s", targetURL, rec.Header().Get("Location"))
	}

	// Verify Cache was populated
	cached, hit := cache.Get(ctx, code)
	if !hit || cached != targetURL {
		t.Errorf("expected cache to be populated after cache miss")
	}

	// Second request: Cache Hit -> Increments click, redirects 307
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected status 307 on cache hit, got %d", rec2.Code)
	}

	// Verify clicks count is now 2
	mapping, _ := repo.GetByShortCode(ctx, code)
	if mapping.Clicks != 2 {
		t.Errorf("expected 2 clicks, got %d", mapping.Clicks)
	}

	// Unknown short code -> 404
	reqUnknown := httptest.NewRequest(http.MethodGet, "/unknown99", nil)
	recUnknown := httptest.NewRecorder()
	handler.ServeHTTP(recUnknown, reqUnknown)
	if recUnknown.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown code, got %d", recUnknown.Code)
	}
}

func TestAnalyticsEndpoint(t *testing.T) {
	handler, repo, _ := setupTestServer()
	ctx := context.Background()

	code := "stats1"
	targetURL := "https://stats.example.com"
	_ = repo.CreateURL(ctx, code, targetURL)
	_ = repo.IncrementClicks(ctx, code)
	_ = repo.IncrementClicks(ctx, code)
	_ = repo.IncrementClicks(ctx, code)

	req := httptest.NewRequest(http.MethodGet, "/analytics/"+code, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp models.AnalyticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode analytics response: %v", err)
	}

	if resp.ShortCode != code {
		t.Errorf("expected short_code %s, got %s", code, resp.ShortCode)
	}
	if resp.LongURL != targetURL {
		t.Errorf("expected long_url %s, got %s", targetURL, resp.LongURL)
	}
	if resp.Clicks != 3 {
		t.Errorf("expected 3 clicks, got %d", resp.Clicks)
	}
	if resp.CreatedAt.IsZero() {
		t.Errorf("expected non-zero created_at")
	}

	// Analytics for non-existent code
	reqUnknown := httptest.NewRequest(http.MethodGet, "/analytics/nonexistent", nil)
	recUnknown := httptest.NewRecorder()
	handler.ServeHTTP(recUnknown, reqUnknown)
	if recUnknown.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent analytics code, got %d", recUnknown.Code)
	}
}

func TestConcurrentRedirects(t *testing.T) {
	handler, repo, _ := setupTestServer()
	ctx := context.Background()

	code := "concur1"
	_ = repo.CreateURL(ctx, code, "https://example.com/concurrent")

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusTemporaryRedirect {
				t.Errorf("unexpected status: %d", rec.Code)
			}
		}()
	}

	wg.Wait()

	mapping, _ := repo.GetByShortCode(ctx, code)
	if mapping.Clicks != int64(goroutines) {
		t.Errorf("expected %d clicks, got %d", goroutines, mapping.Clicks)
	}
}

func TestMiddlewareCORSAndPanicRecovery(t *testing.T) {
	// Test CORS OPTIONS preflight
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/panic" {
			panic("something went catastrophically wrong")
		}
		w.WriteHeader(http.StatusOK)
	})

	chained := Chain(panicHandler, CORS, Recoverer)

	// Options preflight
	reqOpt := httptest.NewRequest(http.MethodOptions, "/test", nil)
	recOpt := httptest.NewRecorder()
	chained.ServeHTTP(recOpt, reqOpt)
	if recOpt.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS preflight, got %d", recOpt.Code)
	}
	if recOpt.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS origin *")
	}

	// Panic recovery
	reqPanic := httptest.NewRequest(http.MethodGet, "/panic", nil)
	recPanic := httptest.NewRecorder()
	chained.ServeHTTP(recPanic, reqPanic)

	if recPanic.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on panic recovery, got %d", recPanic.Code)
	}
	var errResp models.ErrorResponse
	_ = json.Unmarshal(recPanic.Body.Bytes(), &errResp)
	if errResp.Detail != "Internal server error" {
		t.Errorf("expected detail 'Internal server error', got %q", errResp.Detail)
	}
}
