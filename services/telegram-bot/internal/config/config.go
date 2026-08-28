package config

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

type Config struct {
	BotToken         string
	InternalToken    string
	AuthURL          string
	VPSURL           string
	BillingURL       string
	SupportURL       string
	SiteURL          string
	ChannelURL       string
	SupportURLPublic string
	PollTimeout      time.Duration
}

func Load() Config {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_INTERNAL_TOKEN"))
	if token == "" && !prodenv.IsProduction() {
		token = strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_TOKEN"))
	}
	if prodenv.IsProduction() && token == "" {
		log.Fatal("TELEGRAM_INTERNAL_TOKEN must be set when APP_ENV=production")
	}
	return Config{
		BotToken:         env("TELEGRAM_BOT_TOKEN", ""),
		InternalToken:    token,
		AuthURL:          env("AUTH_SERVICE_URL", "http://auth:8001"),
		VPSURL:           env("VPS_SERVICE_URL", "http://vps:8003"),
		BillingURL:       env("BILLING_SERVICE_URL", "http://billing:8002"),
		SupportURL:       env("SUPPORT_SERVICE_URL", "http://support:8005"),
		SiteURL:          strings.TrimRight(env("SITE_URL", "https://cloud-hustle.com"), "/"),
		ChannelURL:       env("TELEGRAM_CHANNEL_URL", "https://t.me/+HbSdIOauph9hMjAy"),
		SupportURLPublic: env("TELEGRAM_SUPPORT_URL", "https://cloud-hustle.com/vps/support"),
		PollTimeout:      durationEnv("TELEGRAM_POLL_TIMEOUT", 30*time.Second),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
