package models

import "time"

// URLMapping represents a shortened URL record in the database.
type URLMapping struct {
	ID        int64     `json:"id"`
	ShortCode string    `json:"short_code"`
	LongURL   string    `json:"long_url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

// ShortenRequest defines the payload for POST /shorten.
type ShortenRequest struct {
	LongURL string `json:"long_url"`
}

// ShortenResponse defines the JSON response for POST /shorten.
type ShortenResponse struct {
	ShortURL string `json:"short_url"`
}

// AnalyticsResponse defines the JSON response for GET /analytics/{short_code}.
type AnalyticsResponse struct {
	ShortCode string    `json:"short_code"`
	LongURL   string    `json:"long_url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrorResponse defines the standard error structure.
type ErrorResponse struct {
	Detail string `json:"detail"`
}
