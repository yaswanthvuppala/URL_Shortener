package db

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
)

func setupTestDB(t *testing.T) (*Database, *Repository) {
	d, err := Connect("sqlite://:memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	repo := NewRepository(d.DB, d.IsPostgres)
	return d, repo
}

func TestRepositoryOperations(t *testing.T) {
	d, repo := setupTestDB(t)
	defer d.Close()

	ctx := context.Background()

	// 1. Check non-existent code
	exists, err := repo.ShortCodeExists(ctx, "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Errorf("expected code to not exist")
	}

	// 2. Create URL
	err = repo.CreateURL(ctx, "abc123", "https://example.com/hello")
	if err != nil {
		t.Fatalf("failed to create URL: %v", err)
	}

	// 3. Check exists now
	exists, err = repo.ShortCodeExists(ctx, "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Errorf("expected code to exist")
	}

	// 4. Retrieve URL
	mapping, err := repo.GetByShortCode(ctx, "abc123")
	if err != nil {
		t.Fatalf("failed to get mapping: %v", err)
	}
	if mapping.ShortCode != "abc123" {
		t.Errorf("expected short_code abc123, got %s", mapping.ShortCode)
	}
	if mapping.LongURL != "https://example.com/hello" {
		t.Errorf("expected long_url https://example.com/hello, got %s", mapping.LongURL)
	}
	if mapping.Clicks != 0 {
		t.Errorf("expected 0 clicks, got %d", mapping.Clicks)
	}
	if mapping.CreatedAt.IsZero() {
		t.Errorf("expected non-zero created_at")
	}

	// 5. Duplicate code should fail (unique constraint)
	err = repo.CreateURL(ctx, "abc123", "https://another.com")
	if err == nil {
		t.Errorf("expected duplicate create to fail, but it succeeded")
	}

	// 6. Non-existent GetByShortCode should return sql.ErrNoRows
	_, err = repo.GetByShortCode(ctx, "nonexistent")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestConcurrentClickIncrements(t *testing.T) {
	d, repo := setupTestDB(t)
	defer d.Close()

	ctx := context.Background()
	code := "counterTest"
	if err := repo.CreateURL(ctx, code, "https://golang.org"); err != nil {
		t.Fatalf("failed to create URL: %v", err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := repo.IncrementClicks(ctx, code); err != nil {
				t.Errorf("concurrent increment failed: %v", err)
			}
		}()
	}

	wg.Wait()

	mapping, err := repo.GetByShortCode(ctx, code)
	if err != nil {
		t.Fatalf("failed to get mapping: %v", err)
	}

	if mapping.Clicks != int64(goroutines) {
		t.Errorf("expected clicks to be %d, got %d (race condition detected!)", goroutines, mapping.Clicks)
	}
}

func TestPostgresRebind(t *testing.T) {
	repo := NewRepository(nil, true)
	query := "SELECT * FROM test WHERE a = ? AND b = ? AND c = ?"
	expected := "SELECT * FROM test WHERE a = $1 AND b = $2 AND c = $3"
	got := repo.rebind(query)
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}

	repoSQLite := NewRepository(nil, false)
	gotSQLite := repoSQLite.rebind(query)
	if gotSQLite != query {
		t.Errorf("expected %s, got %s", query, gotSQLite)
	}
}
