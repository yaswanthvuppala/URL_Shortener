package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yaswanthvuppala/url-shortener/internal/models"
)

func TestGenerateShortCode(t *testing.T) {
	lengths := []int{1, 6, 10, 32}
	for _, l := range lengths {
		code, err := GenerateShortCode(l)
		if err != nil {
			t.Fatalf("unexpected error generating code length %d: %v", l, err)
		}
		if len(code) != l {
			t.Errorf("expected length %d, got %d", l, len(code))
		}
		for _, ch := range code {
			if !strings.ContainsRune(charset, ch) {
				t.Errorf("character %c not in base62 charset", ch)
			}
		}
	}

	// Zero or negative length
	_, err := GenerateShortCode(0)
	if err == nil {
		t.Errorf("expected error for length 0")
	}

	// Ensure consecutive codes are not identical
	code1, _ := GenerateShortCode(6)
	code2, _ := GenerateShortCode(6)
	if code1 == code2 {
		t.Errorf("two consecutive generated codes are identical: %s", code1)
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://google.com", false},
		{"valid http", "http://example.com/path?arg=val", false},
		{"with port", "http://localhost:8000/test", false},
		{"empty string", "", true},
		{"spaces only", "   ", true},
		{"missing scheme", "www.example.com", true},
		{"ftp scheme", "ftp://example.com/file", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"missing host", "http://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ValidateURL(tt.url)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for URL %q, got nil", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected valid URL %q, got error: %v", tt.url, err)
			}
			if !tt.wantErr && parsed == nil {
				t.Errorf("expected non-nil parsed URL for %q", tt.url)
			}
		})
	}
}

func TestRespondJSONAndError(t *testing.T) {
	// Test RespondJSON
	rec := httptest.NewRecorder()
	RespondJSON(rec, http.StatusOK, map[string]string{"message": "hello"})
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json content type, got %s", rec.Header().Get("Content-Type"))
	}

	// Test RespondError
	recErr := httptest.NewRecorder()
	RespondError(recErr, http.StatusNotFound, "not found test")
	if recErr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", recErr.Code)
	}
	var errResp models.ErrorResponse
	if err := json.Unmarshal(recErr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Detail != "not found test" {
		t.Errorf("expected detail 'not found test', got %q", errResp.Detail)
	}
}
