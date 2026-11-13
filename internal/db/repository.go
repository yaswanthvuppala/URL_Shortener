package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yaswanthvuppala/url-shortener/internal/models"
)

// Repository provides data access operations for URL mappings.
type Repository struct {
	db         *sql.DB
	isPostgres bool
}

// NewRepository creates a new Repository instance.
func NewRepository(db *sql.DB, isPostgres bool) *Repository {
	return &Repository{
		db:         db,
		isPostgres: isPostgres,
	}
}

// rebind converts standard '?' placeholders to PostgreSQL '$1', '$2', ... if running on Postgres.
func (r *Repository) rebind(query string) string {
	if !r.isPostgres {
		return query
	}
	var sb strings.Builder
	argIdx := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			sb.WriteString(fmt.Sprintf("$%d", argIdx))
			argIdx++
		} else {
			sb.WriteByte(query[i])
		}
	}
	return sb.String()
}

// CreateURL inserts a new URL mapping with 0 clicks.
func (r *Repository) CreateURL(ctx context.Context, shortCode, longURL string) error {
	query := r.rebind("INSERT INTO url_mappings (short_code, long_url, clicks, created_at) VALUES (?, ?, 0, CURRENT_TIMESTAMP)")
	_, err := r.db.ExecContext(ctx, query, shortCode, longURL)
	return err
}

// GetByShortCode retrieves a URL mapping by its unique short code.
func (r *Repository) GetByShortCode(ctx context.Context, shortCode string) (*models.URLMapping, error) {
	query := r.rebind("SELECT id, short_code, long_url, clicks, created_at FROM url_mappings WHERE short_code = ?")
	row := r.db.QueryRowContext(ctx, query, shortCode)

	var mapping models.URLMapping
	var rawCreatedAt any

	err := row.Scan(&mapping.ID, &mapping.ShortCode, &mapping.LongURL, &mapping.Clicks, &rawCreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to scan url mapping: %w", err)
	}

	mapping.CreatedAt = parseTime(rawCreatedAt)
	return &mapping, nil
}

// IncrementClicks atomically increments the click counter for the given short code.
func (r *Repository) IncrementClicks(ctx context.Context, shortCode string) error {
	query := r.rebind("UPDATE url_mappings SET clicks = clicks + 1 WHERE short_code = ?")
	res, err := r.db.ExecContext(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to increment clicks: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ShortCodeExists checks whether a short code already exists in the database.
func (r *Repository) ShortCodeExists(ctx context.Context, shortCode string) (bool, error) {
	query := r.rebind("SELECT 1 FROM url_mappings WHERE short_code = ? LIMIT 1")
	var dummy int
	err := r.db.QueryRowContext(ctx, query, shortCode).Scan(&dummy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed checking short code existence: %w", err)
	}
	return true, nil
}

// parseTime parses various database timestamp representations into a valid time.Time.
func parseTime(val any) time.Time {
	if val == nil {
		return time.Now().UTC()
	}

	switch v := val.(type) {
	case time.Time:
		return v.UTC()
	case string:
		return parseTimeString(v)
	case []byte:
		return parseTimeString(string(v))
	default:
		return time.Now().UTC()
	}
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func parseTimeString(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}
