// Package cache manages NoxOJ's connection to Redis.
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Host string
	Port int
}

// Connect opens a connection to Redis and verifies it's actually
// reachable (via PING) before returning — same contract as
// internal/database.Connect: a returned client is a promise Redis
// was reachable at least at startup, not just that the address parsed.
func Connect(cfg Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}

	return client, nil
}

// Checker returns a health.Checker reporting whether Redis is
// currently reachable — same role as database.Checker plays for
// Postgres.
func Checker(client *redis.Client) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return client.Ping(ctx).Err()
	}
}
