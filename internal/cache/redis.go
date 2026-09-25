package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"Mogza/TaoDash/internal/types"

	"github.com/redis/go-redis/v9"
)

const safeguardTTL = 5 * time.Minute

// Client wraps a Redis connection.
// Nil-safe: all methods check for nil before operating.
type Client struct {
	rdb *redis.Client
}

// New creates a Redis client from a URL (e.g. "redis://localhost:6379").
// Returns an error on connection failure — caller must handle the fallback.
func New(redisURL string) (*Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis: parse URL: %w", err)
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping failed: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

// Get retrieves cached neurons by an arbitrary string key.
// Returns false if the key is missing or the client is nil.
func (c *Client) Get(ctx context.Context, key string) ([]types.Neuron, bool) {
	if c == nil {
		return nil, false
	}

	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}

	var neurons []types.Neuron
	if err := json.Unmarshal(data, &neurons); err != nil {
		return nil, false
	}

	return neurons, true
}

// Set stores neurons under an arbitrary string key with the safeguard TTL.
func (c *Client) Set(ctx context.Context, key string, neurons []types.Neuron) error {
	if c == nil {
		return nil
	}

	data, err := json.Marshal(neurons)
	if err != nil {
		return fmt.Errorf("redis: marshal neurons: %w", err)
	}

	return c.rdb.Set(ctx, key, data, safeguardTTL).Err()
}

// Del removes a key — used by the stress test to isolate scenarios.
func (c *Client) Del(ctx context.Context, key string) error {
	if c == nil {
		return nil
	}
	return c.rdb.Del(ctx, key).Err()
}
