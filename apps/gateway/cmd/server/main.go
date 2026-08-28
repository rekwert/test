package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/apps/gateway/internal/accesslog"
	"github.com/borishru-boop/testVPStrade/apps/gateway/internal/ratelimit"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/clientip"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/dbpool"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/redis"
)

func main() {
	prodenv.AssertBackGatewayBind()
	ctx := context.Background()
	port := env("GATEWAY_PORT", "8080")
	corsOrigins := parseOrigins(env("CORS_ORIGINS", "http://localhost:3000"))

	authURL := mustURL(env("AUTH_SERVICE_URL", "http://auth:8001"))
	billingURL := mustURL(env("BILLING_SERVICE_URL", "http://billing:8002"))
	vpsURL := mustURL(env("VPS_SERVICE_URL", "http://vps:8003"))
	supportURL := mustURL(env("SUPPORT_SERVICE_URL", "http://support:8005"))
	notificationURL := mustURL(env("NOTIFICATION_SERVICE_URL", "http://notification:8004"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /ready", ready)
	mux.HandleFunc("GET /api/v1/health", health)

	mountProxy(mux, "/api/v1/auth", authURL, "/api/v1/auth")
	mountProxy(mux, "/api/v1/referral", authURL, "/api/v1")
	mountProxy(mux, "/api/v1/billing", billingURL, "/api/v1/billing")
	mountProxy(mux, "/api/v1/webhooks", billingURL, "/api/v1/webhooks")
	mountProxy(mux, "/api/v1/plans", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/catalog", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/orders", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/free-week", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/instance-slug", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/instances", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/clients", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/tools", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/stats", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/instances", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/nodes", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/region-tiers", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/abuse", vpsURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/tickets", supportURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/shift", supportURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/workspace", supportURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/queue", supportURL, "/api/v1")
	mountProxy(mux, "/api/v1/tickets", supportURL, "/api/v1")
	mountProxy(mux, "/api/v1/notifications", notificationURL, "/api/v1")
	mountProxy(mux, "/api/v1/admin/notifications", notificationURL, "/api/v1")

	handler := http.Handler(mux)
	if dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN")); dsn != "" {
		pool, err := dbpool.Open(ctx, dsn)
		if err != nil {
			log.Fatalf("gateway db: %v", err)
		}
		defer pool.Close()
		al := accesslog.New(pool)
		al.Start(ctx)
		defer al.Close()
		handler = al.Middleware(handler)
		log.Printf("gateway: HTTP access log enabled (POSTGRES_DSN)")
	} else if prodenv.IsProduction() {
		log.Printf("gateway: POSTGRES_DSN not set — HTTP access log disabled")
	}

	rlCfg := ratelimit.LoadConfig()
	if rlCfg.Enabled {
		if prodenv.IsProduction() && strings.TrimSpace(env("REDIS_URL", "")) == "" {
			log.Fatal("REDIS_URL must be set when APP_ENV=production and rate limiting is enabled")
		}
		if redisURL := env("REDIS_URL", ""); redisURL != "" {
			rdb, err := redis.New(redisURL)
			if err != nil {
				if prodenv.IsProduction() {
					log.Fatalf("gateway: redis unavailable: %v", err)
				}
				log.Printf("gateway: redis unavailable (%v), rate limiting disabled", err)
			} else {
				handler = ratelimit.New(rdb, rlCfg).Wrap(handler)
				log.Printf("gateway: rate limiting enabled (default=%d/min sensitive=%d/min)",
					rlCfg.DefaultLimit, rlCfg.SensitiveLimit)
			}
		} else if prodenv.IsProduction() {
			handler = ratelimit.New(nil, rlCfg).Wrap(handler)
		}
	}

	handler = withCORS(corsOrigins, handler)
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		// Password reset / VirtFusion queue waits can exceed 30s; console/ws is long-lived.
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("gateway listening on :%s", port)

	tlsCert := strings.TrimSpace(os.Getenv("GATEWAY_TLS_CERT"))
	tlsKey := strings.TrimSpace(os.Getenv("GATEWAY_TLS_KEY"))
	tlsPort := strings.TrimSpace(env("GATEWAY_TLS_PORT", ""))
	if tlsCert != "" && tlsKey != "" && tlsPort != "" {
		if _, err := os.Stat(tlsCert); err != nil {
			log.Printf("gateway TLS disabled: cert not found at %s (%v)", tlsCert, err)
		} else if _, err := os.Stat(tlsKey); err != nil {
			log.Printf("gateway TLS disabled: key not found at %s (%v)", tlsKey, err)
		} else {
			go func() {
				tlsSrv := &http.Server{
					Addr:         ":" + tlsPort,
					Handler:      handler,
					ReadTimeout:  15 * time.Second,
					WriteTimeout: 0,
					IdleTimeout:  120 * time.Second,
				}
				log.Printf("gateway TLS listening on :%s", tlsPort)
				if err := tlsSrv.ListenAndServeTLS(tlsCert, tlsKey); err != nil && err != http.ErrServerClosed {
					log.Printf("gateway TLS stopped: %v", err)
				}
			}()
		}
	}

	log.Fatal(srv.ListenAndServe())
}

func mountProxy(mux *http.ServeMux, publicPrefix string, target *url.URL, stripPrefix string) {
	publicPort := 443
	if p := strings.TrimSpace(env("GATEWAY_PUBLIC_PORT", "443")); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			publicPort = n
		}
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	origDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		origDirector(r)
		ep := clientip.ClientEndpoint(r)
		clientip.SetForwardHeaders(r, ep, publicPort)
		path := r.URL.Path
		if stripPrefix != "" && strings.HasPrefix(path, stripPrefix) {
			path = strings.TrimPrefix(path, stripPrefix)
			if path == "" {
				path = "/"
			}
		}
		r.URL.Path = path
		r.URL.RawPath = ""
		r.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy %s: %v", publicPrefix, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream unavailable"})
	}
	proxy.Transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: 110 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
	longProxy := httputil.NewSingleHostReverseProxy(target)
	longProxy.Director = proxy.Director
	longProxy.ErrorHandler = proxy.ErrorHandler
	longProxy.Transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: 5 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if longLivedRequest(r) {
			longProxy.ServeHTTP(w, r)
			return
		}
		http.TimeoutHandler(proxy, 90*time.Second, `{"error":"gateway timeout"}`).ServeHTTP(w, r)
	})

	mux.Handle(publicPrefix+"/", handler)
	mux.Handle(publicPrefix, handler)
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Connection"), "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func longLivedRequest(r *http.Request) bool {
	if isWebSocketUpgrade(r) {
		return true
	}
	path := r.URL.Path
	if strings.Contains(path, "/console/ws") {
		return true
	}
	// Change-IP runs VF assign + network sync + guest SSH (often >90s).
	if strings.Contains(path, "/change-ip") {
		return true
	}
	return false
}

func withCORS(origins []string, next http.Handler) http.Handler {
	allowAll := len(origins) == 1 && origins[0] == "*"
	if prodenv.IsProduction() && allowAll {
		allowAll = false
		log.Printf("gateway: CORS allow-all disabled in production")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" && originAllowed(origins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-Api-Key, Content-Type, Accept, Accept-Language, X-Session-Id, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func originAllowed(origins []string, origin string) bool {
	for _, o := range origins {
		if o == origin {
			return true
		}
	}
	return false
}

func parseOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "gateway"})
}

func ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("invalid url %q: %v", raw, err)
	}
	return u
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
