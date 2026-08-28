package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/redis"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/config"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/handler"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/ratelimit"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/store"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/tokens"
)

func main() {
	cfg := config.Load()
	if prodenv.IsProduction() && strings.TrimSpace(os.Getenv("TELEGRAM_INTERNAL_TOKEN")) == "" {
		log.Fatal("TELEGRAM_INTERNAL_TOKEN must be set when APP_ENV=production")
	}
	prodenv.AssertPostgresDSNSecurity(cfg.PostgresDSN)
	ctx := context.Background()

	st, err := store.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer st.Close()

	tm := tokens.New(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	nc := notify.New(cfg.NotificationURL, cfg.NotificationToken)

	var limiter *ratelimit.LoginLimiter
	var refreshGrace *ratelimit.RefreshGrace
	if cfg.RedisURL == "" {
		if prodenv.IsProduction() {
			log.Fatal("REDIS_URL must be set when APP_ENV=production (auth rate limiting)")
		}
	} else {
		rdb, err := redis.New(cfg.RedisURL)
		if err != nil {
			if prodenv.IsProduction() {
				log.Fatalf("auth: redis required in production: %v", err)
			}
			log.Printf("auth: redis unavailable (%v), login rate limit disabled", err)
		} else {
			defer rdb.Close()
			limiter = ratelimit.NewLoginLimiter(rdb, 20, 15*time.Minute)
			refreshGrace = ratelimit.NewRefreshGrace(rdb, 45*time.Second)
			log.Println("auth: redis login rate limit enabled")
		}
	}
	if prodenv.IsProduction() && limiter == nil {
		log.Fatal("auth: login rate limiter required in production")
	}

	h := handler.New(st, tm, nc, cfg, limiter, refreshGrace)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "auth"})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /register", h.Register)
	mux.HandleFunc("POST /login", h.Login)
	mux.HandleFunc("POST /refresh", h.Refresh)
	mux.HandleFunc("GET /me", h.Me)
	mux.HandleFunc("PATCH /me", h.UpdateMe)
	mux.HandleFunc("POST /change-password", h.ChangePassword)
	mux.HandleFunc("GET /ssh-keys", h.ListSSHKeys)
	mux.HandleFunc("POST /ssh-keys", h.CreateSSHKey)
	mux.HandleFunc("DELETE /ssh-keys/{id}", h.DeleteSSHKey)
	mux.HandleFunc("GET /api-keys", h.ListAPIKeys)
	mux.HandleFunc("POST /api-keys", h.CreateAPIKey)
	mux.HandleFunc("DELETE /api-keys/{id}", h.DeleteAPIKey)
	mux.HandleFunc("GET /admin/users/{id}/api-keys", h.AdminListUserAPIKeys)
	mux.HandleFunc("POST /admin/users/{id}/api-keys", h.AdminCreateUserAPIKey)
	mux.HandleFunc("DELETE /admin/users/{id}/api-keys/{key_id}", h.AdminRevokeUserAPIKey)
	mux.HandleFunc("POST /logout", h.Logout)
	mux.HandleFunc("GET /sessions", h.ListSessions)
	mux.HandleFunc("DELETE /sessions/others", h.RevokeOtherSessions)
	mux.HandleFunc("DELETE /sessions/{id}", h.RevokeSession)
	mux.HandleFunc("GET /admin/users", h.AdminListUsers)
	mux.HandleFunc("GET /admin/users/{id}", h.AdminGetUser)
	mux.HandleFunc("DELETE /admin/users/{id}", h.AdminDeleteUser)
	mux.HandleFunc("PATCH /admin/users/{id}/roles", h.AdminSetUserRoles)
	mux.HandleFunc("POST /admin/users/{id}/impersonate", h.AdminImpersonateUser)
	mux.HandleFunc("GET /admin/users/{id}/referrals", h.AdminGetUserReferrals)
	mux.HandleFunc("POST /admin/users/{id}/referral", h.AdminAssignReferral)
	mux.HandleFunc("GET /admin/audit", h.AdminListAudit)
	mux.HandleFunc("POST /verify-email", h.VerifyEmail)
	mux.HandleFunc("POST /resend-verification", h.ResendVerification)
	mux.HandleFunc("POST /forgot-password", h.ForgotPassword)
	mux.HandleFunc("POST /reset-password", h.ResetPassword)
	mux.HandleFunc("GET /referral", h.ReferralDashboard)
	mux.HandleFunc("POST /referral/click", h.ReferralClick)
	mux.HandleFunc("POST /telegram/link/request", h.TelegramLinkRequest)
	mux.HandleFunc("POST /telegram/link/confirm", h.TelegramLinkConfirm)
	mux.HandleFunc("POST /telegram/link/web", h.TelegramWebLinkStart)
	mux.HandleFunc("POST /telegram/link/web/confirm", h.TelegramWebLinkConfirm)
	mux.HandleFunc("GET /telegram/by-id/{telegram_id}", h.TelegramResolve)
	mux.HandleFunc("POST /telegram/bot-session", h.TelegramBotSession)
	mux.HandleFunc("DELETE /telegram/link", h.TelegramUnlink)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("auth service listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
