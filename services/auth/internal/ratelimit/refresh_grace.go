package ratelimit

import (
	"context"
	"fmt"
	"time"

	sharedredis "github.com/borishru-boop/testVPStrade/packages/shared-go/redis"
)

// RefreshGrace allows parallel refresh requests (multi-tab) right after rotation.
type RefreshGrace struct {
	redis *sharedredis.Client
	ttl   time.Duration
}

func NewRefreshGrace(rdb *sharedredis.Client, ttl time.Duration) *RefreshGrace {
	if ttl <= 0 {
		ttl = 45 * time.Second
	}
	return &RefreshGrace{redis: rdb, ttl: ttl}
}

func (g *RefreshGrace) Remember(ctx context.Context, consumedHash, userID string) {
	if g == nil || g.redis == nil || consumedHash == "" || userID == "" {
		return
	}
	key := fmt.Sprintf("auth:refresh:grace:%s", consumedHash)
	_ = g.redis.Raw().Set(ctx, key, userID, g.ttl).Err()
}

func (g *RefreshGrace) UserID(ctx context.Context, consumedHash string) (string, bool) {
	if g == nil || g.redis == nil || consumedHash == "" {
		return "", false
	}
	key := fmt.Sprintf("auth:refresh:grace:%s", consumedHash)
	val, err := g.redis.Raw().Get(ctx, key).Result()
	if err != nil || val == "" {
		return "", false
	}
	return val, true
}
