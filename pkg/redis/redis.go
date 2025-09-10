package redis

import (
	"context"
	"fmt"

	"payment-system/internal/config"

	"github.com/redis/go-redis/v9"
)

// Client wrapper para cliente Redis
type Client struct {
	*redis.Client
}

// New crea un nuevo cliente Redis
func New(cfg config.RedisConfig) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.GetRedisAddr(),
		Password:     cfg.Password,
		DB:           cfg.Database,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// Verificar conexión
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Client{rdb}, nil
}

// Close cierra la conexión Redis
func (c *Client) Close() error {
	return c.Client.Close()
}
