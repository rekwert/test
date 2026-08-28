package ratelimit

import (
	"context"
	"fmt"
	"time"

	sharedredis "github.com/borishru-boop/testVPStrade/packages/shared-go/redis"
)

type LoginLimiter struct {
	redis *sharedredis.Client
	limit int
	window time.Duration
}

func NewLoginLimiter(rdb *sharedredis.Client, limit int, window time.Duration) *LoginLimiter {
	if limit <= 0 {
		limit = 20
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	return &LoginLimiter{redis: rdb, limit: limit, window: window}
}

func (l *LoginLimiter) Allow(ctx context.Context, ip, email string) (bool, error) {
	if l == nil || l.redis == nil {
		return true, nil
	}
	key := fmt.Sprintf("auth:login:%s:%s", ip, email)
	return l.redis.Allow(ctx, key, l.limit, l.window)
}

func (l *LoginLimiter) AllowRegister(ctx context.Context, ip string) (bool, error) {
	if l == nil || l.redis == nil {
		return true, nil
	}
	key := fmt.Sprintf("auth:register:%s", ip)
	return l.redis.Allow(ctx, key, 10, time.Hour)
}

func (l *LoginLimiter) AllowForgotPassword(ctx context.Context, ip string) (bool, error) {
	if l == nil || l.redis == nil {
		return true, nil
	}
	key := fmt.Sprintf("auth:forgot:%s", ip)
	return l.redis.Allow(ctx, key, 5, time.Hour)
}
