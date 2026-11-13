package cache

import (
	"context"
	"testing"
	"time"
)

func TestNilRedisCache(t *testing.T) {
	c := &RedisCache{client: nil}

	ctx := context.Background()
	val, ok := c.Get(ctx, "testkey")
	if ok || val != "" {
		t.Errorf("expected empty result and false from nil client, got val=%q ok=%v", val, ok)
	}

	// Should not panic
	c.Set(ctx, "testkey", "val", time.Hour)

	if err := c.Close(); err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

func TestUnreachableRedisCache(t *testing.T) {
	// Point to non-existent local port
	c := NewRedisCache("redis://127.0.0.1:59999")
	defer c.Close()

	ctx := context.Background()

	// Cache GET should fail gracefully to DB without error or panic
	val, ok := c.Get(ctx, "anykey")
	if ok || val != "" {
		t.Errorf("expected ok=false for unreachable Redis, got ok=%v", ok)
	}

	// Cache SET should log warning and not panic
	c.Set(ctx, "anykey", "val", time.Minute)
}

func TestEmptyRedisURL(t *testing.T) {
	c := NewRedisCache("")
	defer c.Close()

	ctx := context.Background()
	val, ok := c.Get(ctx, "anykey")
	if ok || val != "" {
		t.Errorf("expected ok=false for empty Redis URL")
	}
}
