package ratelimit

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	sharedredis "github.com/borishru-boop/testVPStrade/packages/shared-go/redis"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

type Config struct {
	Enabled         bool
	DefaultLimit    int
	DefaultWindow   time.Duration
	AuthLimit       int
	AuthWindow      time.Duration
	SensitiveLimit  int
	SensitiveWindow time.Duration
}

func LoadConfig() Config {
	return Config{
		Enabled:         boolEnv("RATE_LIMIT_ENABLED", true),
		DefaultLimit:    intEnv("RATE_LIMIT_DEFAULT", 300),
		DefaultWindow:   durationEnv("RATE_LIMIT_WINDOW", time.Minute),
		AuthLimit:       intEnv("RATE_LIMIT_AUTH", 120),
		AuthWindow:      durationEnv("RATE_LIMIT_AUTH_WINDOW", time.Minute),
		SensitiveLimit:  intEnv("RATE_LIMIT_SENSITIVE", 15),
		SensitiveWindow: durationEnv("RATE_LIMIT_SENSITIVE_WINDOW", time.Minute),
	}
}

type Middleware struct {
	redis *sharedredis.Client
	cfg   Config
}

func New(rdb *sharedredis.Client, cfg Config) *Middleware {
	return &Middleware{redis: rdb, cfg: cfg}
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	if m == nil || !m.cfg.Enabled {
		return next
	}
	if m.redis == nil {
		if prodenv.IsProduction() {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodOptions {
					next.ServeHTTP(w, r)
					return
				}
				path := r.URL.Path
				if path == "/health" || path == "/ready" || path == "/api/v1/health" {
					next.ServeHTTP(w, r)
					return
				}
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rate limit unavailable"})
			})
		}
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		if path == "/health" || path == "/ready" || path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		limit, window := m.limitsFor(path)
		if limit <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		ip := clientIP(r)
		key := fmt.Sprintf("gw:rl:%s:%s", bucketName(path), ip)
		ok, err := m.redis.Allow(r.Context(), key, limit, window)
		if err != nil {
			log.Printf("gateway rate limit: %v", err)
			if prodenv.IsProduction() {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rate limit unavailable"})
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) limitsFor(path string) (int, time.Duration) {
	if strings.Contains(path, "/webhooks/") {
		return 0, 0
	}
	switch {
	case strings.Contains(path, "/auth/login"),
		strings.Contains(path, "/auth/register"),
		strings.Contains(path, "/auth/forgot-password"),
		strings.Contains(path, "/auth/resend-verification"),
		strings.Contains(path, "/auth/reset-password"),
		strings.Contains(path, "/auth/api-keys"):
		return m.cfg.SensitiveLimit, m.cfg.SensitiveWindow
	case strings.Contains(path, "/auth/"):
		return m.cfg.AuthLimit, m.cfg.AuthWindow
	default:
		return m.cfg.DefaultLimit, m.cfg.DefaultWindow
	}
}

func bucketName(path string) string {
	if strings.Contains(path, "/auth/login") {
		return "login"
	}
	if strings.Contains(path, "/auth/register") {
		return "register"
	}
	if strings.Contains(path, "/auth/") {
		return "auth"
	}
	return "api"
}

func clientIP(r *http.Request) string {
	if trustedProxyIP(r.RemoteAddr) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
		if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
			return xri
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func trustedProxyIP(remoteAddr string) bool {
	raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_IPS"))
	if raw == "" {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	for _, p := range strings.Split(raw, ",") {
		if strings.TrimSpace(p) == host {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func boolEnv(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes"
}

func intEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
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
