package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/borishru-boop/testVPStrade/services/telegram-bot/internal/bot"
	"github.com/borishru-boop/testVPStrade/services/telegram-bot/internal/config"
)

func main() {
	cfg := config.Load()
	if cfg.BotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	b := bot.New(cfg)
	log.Printf("telegram-bot starting (site=%s)", cfg.SiteURL)
	if err := b.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("telegram-bot stopped: %v", err)
		os.Exit(1)
	}
}
