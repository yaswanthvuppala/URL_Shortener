package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yaswanthvuppala/url-shortener/internal/cache"
	"github.com/yaswanthvuppala/url-shortener/internal/config"
	"github.com/yaswanthvuppala/url-shortener/internal/db"
	"github.com/yaswanthvuppala/url-shortener/internal/handlers"
	"github.com/yaswanthvuppala/url-shortener/static"
)

func main() {
	cfg := config.Load()

	log.Printf("Starting URL Shortener service on port %s...", cfg.Port)
	log.Printf("Base URL: %s", cfg.BaseURL)

	// Initialize Database
	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Fatal: Database initialization failed: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()
	log.Println("Database initialized and schema verified successfully.")

	// Initialize Cache (resilient to offline Redis)
	redisCache := cache.NewRedisCache(cfg.RedisURL)
	defer func() {
		if err := redisCache.Close(); err != nil {
			log.Printf("Error closing cache: %v", err)
		}
	}()

	// Initialize Handlers & Routes
	repo := db.NewRepository(database.DB, database.IsPostgres)
	appHandler := handlers.NewHandler(repo, redisCache, cfg.BaseURL, static.Files)

	mux := appHandler.Routes()
	wrappedHandler := handlers.Chain(mux, handlers.CORS, handlers.Logger, handlers.Recoverer)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      wrappedHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Channel to catch interrupt/shutdown signals
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Server listening on http://0.0.0.0:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		log.Fatalf("Fatal: Server error: %v", err)
	case sig := <-shutdown:
		log.Printf("Received signal %v. Initiating graceful shutdown...", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Graceful shutdown timed out, forcing server close: %v", err)
			_ = server.Close()
		}
		log.Println("Server stopped gracefully.")
	}
}
