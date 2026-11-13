package handlers

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/yaswanthvuppala/url-shortener/internal/models"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GenerateShortCode generates a cryptographically secure random base62 string of the given length.
func GenerateShortCode(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("length must be greater than zero")
	}

	result := make([]byte, length)
	// Rejection sampling to prevent modulo bias (256 - 256%62 = 248)
	maxVal := byte(256 - (256 % len(charset)))
	b := make([]byte, 1)

	for i := 0; i < length; {
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("failed to read secure random bytes: %w", err)
		}
		if b[0] < maxVal {
			result[i] = charset[int(b[0])%len(charset)]
			i++
		}
	}

	return string(result), nil
}

// ValidateURL validates that the raw URL is well-formed, contains a host, and has an http or https scheme.
func ValidateURL(rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, errors.New("URL cannot be empty")
	}

	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil {
		return nil, errors.New("invalid URL format")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("URL scheme must be http or https")
	}

	if parsed.Host == "" {
		return nil, errors.New("URL must specify a host")
	}

	return parsed, nil
}

// RespondJSON sends a JSON response with the provided HTTP status code.
func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// RespondError sends a standardized JSON error response.
func RespondError(w http.ResponseWriter, status int, detail string) {
	RespondJSON(w, status, models.ErrorResponse{Detail: detail})
}
