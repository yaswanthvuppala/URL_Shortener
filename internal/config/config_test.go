package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	// Clear relevant env vars
	os.Unsetenv("PORT")
	os.Unsetenv("BASE_URL")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("REDIS_URL")

	cfg := Load()

	if cfg.Port != "8000" {
		t.Errorf("expected Port 8000, got %s", cfg.Port)
	}
	if cfg.BaseURL != "http://localhost:8000" {
		t.Errorf("expected BaseURL http://localhost:8000, got %s", cfg.BaseURL)
	}
	if cfg.DatabaseURL != "sqlite:///./url_shortener.db" {
		t.Errorf("expected DatabaseURL sqlite:///./url_shortener.db, got %s", cfg.DatabaseURL)
	}
	if cfg.RedisURL != "redis://localhost:6379" {
		t.Errorf("expected RedisURL redis://localhost:6379, got %s", cfg.RedisURL)
	}
}

func TestPostgresURLRewrite(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/testdb")
	defer os.Unsetenv("DATABASE_URL")

	cfg := Load()
	expected := "postgresql://user:pass@localhost:5432/testdb"
	if cfg.DatabaseURL != expected {
		t.Errorf("expected DatabaseURL %s, got %s", expected, cfg.DatabaseURL)
	}
}

func TestBaseURLTrailingSlash(t *testing.T) {
	os.Setenv("BASE_URL", "https://short.example.com/")
	defer os.Unsetenv("BASE_URL")

	cfg := Load()
	expected := "https://short.example.com"
	if cfg.BaseURL != expected {
		t.Errorf("expected BaseURL %s, got %s", expected, cfg.BaseURL)
	}
}

func TestLoadDotEnv(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	content := `
# Comment line
TEST_CFG_PORT=9090
TEST_CFG_URL="https://custom.domain.com/"
TEST_CFG_QUOTE='single_quoted'
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp .env: %v", err)
	}

	loadDotEnv(envPath)
	defer func() {
		os.Unsetenv("TEST_CFG_PORT")
		os.Unsetenv("TEST_CFG_URL")
		os.Unsetenv("TEST_CFG_QUOTE")
	}()

	if val := os.Getenv("TEST_CFG_PORT"); val != "9090" {
		t.Errorf("expected 9090, got %s", val)
	}
	if val := os.Getenv("TEST_CFG_URL"); val != "https://custom.domain.com/" {
		t.Errorf("expected unquoted URL, got %s", val)
	}
	if val := os.Getenv("TEST_CFG_QUOTE"); val != "single_quoted" {
		t.Errorf("expected single_quoted, got %s", val)
	}
}
