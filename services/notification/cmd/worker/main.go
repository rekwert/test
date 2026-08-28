package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/redis"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/config"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/mailer"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/store"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/worker"
)

func main() {
	cfg := config.Load()
	prodenv.AssertPostgresDSNSecurity(cfg.PostgresDSN)
	if cfg.PostgresDSN == "" {
		log.Fatal("POSTGRES_DSN is required for notification worker")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	st, err := store.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("notification worker store: %v", err)
	}
	defer st.Close()

	m := mailer.New(mailer.Config{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		User:     cfg.SMTPUser,
		Pass:     cfg.SMTPPass,
		From:     cfg.From,
		FromName: cfg.FromName,
		ReplyTo:  cfg.ReplyTo,
		TLS:      cfg.SMTPTLS,
		SSL:      cfg.SMTPSSL,
	})

	var rdb *redis.Client
	if cfg.RedisURL != "" {
		rdb, err = redis.New(cfg.RedisURL)
		if err != nil {
			log.Printf("notification worker: redis unavailable (%v), polling only", err)
		} else {
			defer rdb.Close()
		}
	}

	interval := envDuration("NOTIFICATION_WORKER_INTERVAL", 5*time.Second)
	log.Printf("notification worker started (interval=%s redis=%v smtp=%s)", interval, rdb != nil, m.Mode())

	proc := worker.NewEmailProcessor(st, m, rdb)
	proc.Run(ctx, interval)
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
