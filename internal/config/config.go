package config

import (
	"bufio"
	"os"
	"strings"
)

// Config holds runtime configuration values.
type Config struct {
	Port        string
	BaseURL     string
	DatabaseURL string
	RedisURL    string
}

// Load reads configuration from environment variables (and optional .env file)
// and returns a populated Config struct with appropriate defaults.
func Load() *Config {
	loadDotEnv(".env")

	port := getEnv("PORT", "8000")
	baseURL := getEnv("BASE_URL", "http://localhost:8000")
	databaseURL := getEnv("DATABASE_URL", "sqlite:///./url_shortener.db")
	redisURL := getEnv("REDIS_URL", "redis://localhost:6379")

	// Ensure postgres:// is rewritten to postgresql:// if present
	if strings.HasPrefix(databaseURL, "postgres://") {
		databaseURL = "postgresql://" + strings.TrimPrefix(databaseURL, "postgres://")
	}

	// Remove trailing slash from BaseURL
	baseURL = strings.TrimRight(baseURL, "/")

	return &Config{
		Port:        port,
		BaseURL:     baseURL,
		DatabaseURL: databaseURL,
		RedisURL:    redisURL,
	}
}

// getEnv retrieves the value of the environment variable named by key,
// or returns fallback if the variable is unset or empty.
func getEnv(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

// loadDotEnv loads key=value pairs from the given file into environment variables
// without overwriting existing environment variables.
func loadDotEnv(filepath string) {
	file, err := os.Open(filepath)
	if err != nil {
		return // optional file, ignore error
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Strip optional surrounding single or double quotes
		if len(value) >= 2 {
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
				value = value[1 : len(value)-1]
			}
		}

		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}
