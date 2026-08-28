package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

type Config struct {
	Port               string
	JWTSecret          string
	PostgresDSN        string
	ServiceToken       string
	AttachmentsDir     string
	MaxAttachmentBytes int64
}

func Load() Config {
	token := strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_TOKEN"))
	if token == "" {
		if prodenv.IsProduction() {
			log.Fatal("NOTIFICATION_SERVICE_TOKEN must be set when APP_ENV=production")
		}
		token = "dev-internal-token"
	}
	return Config{
		Port:               env("PORT", "8005"),
		JWTSecret:          prodenv.RequireJWTSecret("dev-secret"),
		PostgresDSN:        env("POSTGRES_DSN", ""),
		ServiceToken:       token,
		AttachmentsDir:     env("ATTACHMENTS_DIR", "/data/attachments"),
		MaxAttachmentBytes: envInt64("MAX_ATTACHMENT_BYTES", 10*1024*1024),
	}
}

func envInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int64
	if _, err := fmt.Sscan(v, &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
