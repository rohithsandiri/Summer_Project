// internal/shared/redis/redis.go
//
// WHY THIS EXISTS:
//   Distributed microservices require a shared fast cache/store for state
//   sharing and coordination. We use Redis for:
//     1. Request Idempotency: Prevent processing duplicate requests (e.g. double orders).
//     2. Cache: Cache SKU metadata/stock to reduce read pressure on databases.
//
// DESIGN:
//   This package initializes the Redis connection, runs a startup Ping check,
//   and exposes simple key-value interfaces.

package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	redisv9 "github.com/redis/go-redis/v9"
)

var (
	// RedisHits counts successful cache lookups.
	RedisHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_cache_hits_total",
			Help: "Total number of successful cache reads.",
		},
		[]string{"service", "cache_name"},
	)

	// RedisMisses counts failed cache lookups.
	RedisMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_cache_misses_total",
			Help: "Total number of cache reads that resulted in a miss.",
		},
		[]string{"service", "cache_name"},
	)
)

// Client wraps the underlying redis client with telemetry and helpers.
type Client struct {
	rdb     *redisv9.Client
	service string
}

// NewClient establishes a connection to Redis and validates it with a ping.
func NewClient(addr, password string, db int, service string) (*Client, error) {
	rdb := redisv9.NewClient(&redisv9.Options{
		Addr:     addr,
		Password: password, // Empty string if no password
		DB:       db,       // 0 for default
		PoolSize: 10,
	})

	// Fail fast ping check
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis at %s: %w", addr, err)
	}

	return &Client{
		rdb:     rdb,
		service: service,
	}, nil
}

// Close closes the underlying Redis client connection pool.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// Get retrieves a value and records cache hit/miss metrics.
func (c *Client) Get(ctx context.Context, cacheName, key string) (string, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redisv9.Nil {
		RedisMisses.WithLabelValues(c.service, cacheName).Inc()
		return "", nil // Return empty string with no error to indicate miss
	}
	if err != nil {
		return "", err
	}

	RedisHits.WithLabelValues(c.service, cacheName).Inc()
	return val, nil
}

// Set stores a value with an expiration TTL.
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiration).Err()
}

// SetNX stores a value if the key does not already exist (Atomic Set-If-Not-Exists).
// Used for distributed locks and request idempotency.
func (c *Client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, key, value, expiration).Result()
}

// Delete removes a key from Redis.
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// Ping checks the connectivity to the Redis instance.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}
