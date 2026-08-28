package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

type Client struct {
	rdb *goredis.Client
}

func New(url string) (*Client, error) {
	if url == "" {
		return nil, fmt.Errorf("redis url is empty")
	}
	opt, err := goredis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	if opt.DialTimeout == 0 {
		opt.DialTimeout = 3 * time.Second
	}
	if opt.ReadTimeout == 0 {
		opt.ReadTimeout = 3 * time.Second
	}
	if opt.WriteTimeout == 0 {
		opt.WriteTimeout = 3 * time.Second
	}
	rdb := goredis.NewClient(opt)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return &Client{rdb: rdb}, nil
}

func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

func (c *Client) Raw() *goredis.Client {
	return c.rdb
}

// Allow fixed-window rate limit. Returns true when under limit.
func (c *Client) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	count, err := c.rdb.Incr(ctx, key).Result()
	if err != nil {
		if prodenv.IsProduction() {
			return false, err
		}
		return true, err
	}
	if count == 1 {
		_ = c.rdb.Expire(ctx, key, window).Err()
	}
	return count <= int64(limit), nil
}

func (c *Client) Publish(ctx context.Context, channel, message string) error {
	return c.rdb.Publish(ctx, channel, message).Err()
}

func (c *Client) Subscribe(ctx context.Context, channels ...string) *goredis.PubSub {
	return c.rdb.Subscribe(ctx, channels...)
}
