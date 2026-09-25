package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"Mogza/TaoDash/internal/types"

	"github.com/redis/go-redis/v9"
)

const (
	// TTL de sécurité : nettoyage automatique si le block watcher plante.
	// En conditions normales, l'invalidation se fait par numéro de bloc.
	safeguardTTL = 5 * time.Minute
)

// Client wraps a Redis connection.
// Nil-safe : toutes les méthodes vérifient si le client est nil avant d'opérer.
type Client struct {
	rdb *redis.Client
}

// New crée un client Redis depuis une URL (ex: "redis://localhost:6379").
// Retourne une erreur si la connexion échoue — l'appelant doit gérer le fallback.
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

// GetMetagraph retourne les neurons cachés pour un subnet + numéro de bloc.
// Retourne false si le cache est vide ou si le client est nil.
func (c *Client) GetMetagraph(ctx context.Context, netUID, blockNumber int) ([]types.Neuron, bool) {
	if c == nil {
		return nil, false
	}

	key := metagraphKey(netUID, blockNumber)
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

// SetMetagraph stocke les neurons pour un subnet + numéro de bloc avec un TTL de sécurité.
func (c *Client) SetMetagraph(ctx context.Context, netUID, blockNumber int, neurons []types.Neuron) error {
	if c == nil {
		return nil
	}

	data, err := json.Marshal(neurons)
	if err != nil {
		return fmt.Errorf("redis: marshal neurons: %w", err)
	}

	return c.rdb.Set(ctx, metagraphKey(netUID, blockNumber), data, safeguardTTL).Err()
}

// Delete supprime une clé de cache — utilisé par le stress test pour isoler les scénarios.
func (c *Client) Delete(ctx context.Context, netUID, blockNumber int) error {
	if c == nil {
		return nil
	}
	return c.rdb.Del(ctx, metagraphKey(netUID, blockNumber)).Err()
}

func metagraphKey(netUID, blockNumber int) string {
	return fmt.Sprintf("taodash:metagraph:%d:%d", netUID, blockNumber)
}
