package dbpool

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open parses DSN and applies PG_POOL_MAX_CONNS / PG_POOL_MIN_CONNS (defaults 8 / 1).
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if n := intEnv("PG_POOL_MAX_CONNS", 8); n > 0 {
		cfg.MaxConns = int32(n)
	}
	if n := intEnv("PG_POOL_MIN_CONNS", 1); n > 0 {
		cfg.MinConns = int32(n)
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	return pgxpool.NewWithConfig(ctx, cfg)
}

func intEnv(key string, def int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	return n
}
