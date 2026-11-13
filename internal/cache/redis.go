package cache

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cacher defines the caching interface used by handlers.
type Cacher interface {
	Get(ctx context.Context, key string) (string, bool)
	Set(ctx context.Context, key string, value string, ttl time.Duration)
	Close() error
}

// RedisCache wraps a Redis client with resilient fallback behavior.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new RedisCache instance.
// If Redis is not available, it logs a warning and returns a cache instance
// that degrades gracefully without crashing the application.
func NewRedisCache(redisURL string) *RedisCache {
	if redisURL == "" {
		log.Println("Redis URL is empty. Operating in database-only mode.")
		return &RedisCache{client: nil}
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("Warning: Failed to parse Redis URL (%s): %v. Proceeding without cache.", redisURL, err)
		return &RedisCache{client: nil}
	}

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Redis unreachable at %s: %v. Running in graceful fallback mode (DB only).", opt.Addr, err)
	} else {
		log.Printf("Connected successfully to Redis at %s", opt.Addr)
	}

	return &RedisCache{client: client}
}

// Get attempts to retrieve a value from Redis.
// Returns (value, true) on cache hit.
// If the key is not found, or if Redis is offline/fails, it returns ("", false)
// so the caller can seamlessly fall back to the database.
func (c *RedisCache) Get(ctx context.Context, key string) (string, bool) {
	if c == nil || c.client == nil {
		return "", false
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()

	val, err := c.client.Get(ctxTimeout, key).Result()
	if err != nil {
		if err != redis.Nil {
			log.Printf("Redis GET error for key '%s': %v (falling back to database)", key, err)
		}
		return "", false
	}
	return val, true
}

// Set stores a key-value pair in Redis with a TTL.
// If Redis is offline or fails, the error is logged without failing the request.
func (c *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) {
	if c == nil || c.client == nil {
		return
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()

	if err := c.client.Set(ctxTimeout, key, value, ttl).Err(); err != nil {
		log.Printf("Redis SET error for key '%s': %v (cache write skipped)", key, err)
	}
}

// Close closes the Redis client connection.
func (c *RedisCache) Close() error {
	if c != nil && c.client != nil {
		return c.client.Close()
	}
	return nil
}
