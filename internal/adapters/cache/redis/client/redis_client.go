package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

type RedisClient struct {
	client *redis.Client
}

func NewRedisClient(cfg config.RedisConfig) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:           fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:       cfg.Password,
		DB:             cfg.DB,
		MaxActiveConns: cfg.MaxConnections,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Connected to Redis successfully")

	return &RedisClient{
		client: rdb,
	}, nil
}

func (r *RedisClient) Close() error {
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("failed to close Redis connection: %w", err)
	}

	logger.Info("Closed Redis connection successfully")
	return nil
}
