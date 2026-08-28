package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

type Config struct {
	Port             string
	PostgresDSN      string
	JWTSecret        string
	AccessTTL        time.Duration
	RefreshTTL       time.Duration
	NotificationURL  string
	NotificationToken string
	VerificationTTL  time.Duration
	PasswordResetTTL time.Duration
	ReferralBaseURL  string
	RedisURL         string

	// First-N registrations promo → ops Telegram alert on each signup.
	RegistrationPromoLimit     int
	RegistrationPromoBonusRub  int
	RegistrationPromoStartedAt time.Time // zero = count all non-deleted users

	// Domains blocked for NEW registrations only (login of existing users still works).
	BlockedEmailDomains []string
}

func Load() Config {
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	if dsn == "" {
		if prodenv.IsProduction() {
			log.Fatal("POSTGRES_DSN must be set when APP_ENV=production")
		}
		dsn = "postgres://vps:changeme@localhost:5432/vps_platform?sslmode=disable"
	}
	notifToken := prodenv.RequireInternalToken("NOTIFICATION_SERVICE_TOKEN", "notification")
	return Config{
		Port:                       env("PORT", "8001"),
		PostgresDSN:                dsn,
		JWTSecret:                  prodenv.RequireJWTSecret("dev-secret-change-me"),
		AccessTTL:                  durationEnv("JWT_ACCESS_TTL", 15*time.Minute),
		RefreshTTL:                 durationEnv("JWT_REFRESH_TTL", 168*time.Hour),
		NotificationURL:            env("NOTIFICATION_SERVICE_URL", "http://notification:8004"),
		NotificationToken:          notifToken,
		VerificationTTL:            durationEnv("EMAIL_CODE_TTL", 15*time.Minute),
		PasswordResetTTL:           durationEnv("PASSWORD_RESET_TTL", 15*time.Minute),
		ReferralBaseURL:            env("REFERRAL_BASE_URL", "https://cloud-hustle.com"),
		RedisURL:                   env("REDIS_URL", ""),
		RegistrationPromoLimit:     intEnv("REGISTRATION_PROMO_LIMIT", 200),
		RegistrationPromoBonusRub:  intEnv("REGISTRATION_PROMO_BONUS_RUB", 500),
		RegistrationPromoStartedAt: timeEnv("REGISTRATION_PROMO_STARTED_AT", time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)),
		BlockedEmailDomains:        csvEnv("BLOCKED_EMAIL_DOMAINS", "proton.me,protonmail.com,protonmail.ch,pm.me"),
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

func intEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// csvEnv returns lowercased trimmed domains. Empty env keeps fallback list.
// Set BLOCKED_EMAIL_DOMAINS to a single dash "-" to disable the blocklist.
func csvEnv(key, fallback string) []string {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		raw = fallback
	}
	if strings.TrimSpace(raw) == "-" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		d := strings.ToLower(strings.TrimSpace(p))
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// timeEnv accepts RFC3339 or YYYY-MM-DD (UTC midnight). Empty → fallback.
func timeEnv(key string, fallback time.Time) time.Time {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC()
	}
	if t, err := time.ParseInLocation("2006-01-02", v, time.UTC); err == nil {
		return t
	}
	return fallback
}
