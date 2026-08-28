package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

type Config struct {
	Port               string
	PostgresDSN        string
	JWTSecret          string
	SMTPHost           string
	SMTPPort           string
	SMTPUser           string
	SMTPPass           string
	SMTPTLS            bool
	SMTPSSL            bool
	From                 string
	FromName             string
	ReplyTo              string
	RedisURL             string
	TelegramBotToken   string
	TelegramAlertsToken string
	TelegramAlertsChatID int64
}

func Load() Config {
	userTok := env("TELEGRAM_BOT_TOKEN", "")
	alertsTok := env("TELEGRAM_ALERTS_BOT_TOKEN", "")
	if alertsTok == "" {
		// Prefer monitoring bot token name used in cloud-hustle-monitoring.
		alertsTok = env("TELEGRAM_ALERTS_TOKEN", "")
	}
	if alertsTok == "" {
		alertsTok = userTok
	}
	chatRaw := env("TELEGRAM_ALERTS_CHAT_ID", "")
	if chatRaw == "" {
		chatRaw = env("TELEGRAM_CHAT_ID", "") // same key as monitoring .env
	}
	var chatID int64
	if chatRaw != "" {
		chatID, _ = strconv.ParseInt(strings.TrimSpace(chatRaw), 10, 64)
	}

	return Config{
		Port:                 env("PORT", "8004"),
		PostgresDSN:          env("POSTGRES_DSN", ""),
		JWTSecret:            prodenv.RequireJWTSecret("dev-secret"),
		SMTPHost:             env("SMTP_HOST", "mailpit"),
		SMTPPort:             env("SMTP_PORT", "1025"),
		SMTPUser:             env("SMTP_USER", ""),
		SMTPPass:             env("SMTP_PASS", ""),
		SMTPTLS:              boolEnv("SMTP_TLS", false),
		SMTPSSL:              boolEnv("SMTP_SSL", false),
		From:                 env("SMTP_FROM", "noreply@vps.local"),
		FromName:             env("SMTP_FROM_NAME", "CLOUD HUSTLE"),
		ReplyTo:              env("SMTP_REPLY_TO", env("SUPPORT_EMAIL", "support@cloud-hustle.com")),
		RedisURL:             env("REDIS_URL", ""),
		TelegramBotToken:     userTok,
		TelegramAlertsToken:  alertsTok,
		TelegramAlertsChatID: chatID,
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
