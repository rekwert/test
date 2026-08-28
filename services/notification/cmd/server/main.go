package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/redis"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/config"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/handler"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/mailer"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/store"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/telegram"
)

func main() {
	cfg := config.Load()
	prodenv.AssertPostgresDSNSecurity(cfg.PostgresDSN)
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

	var st *store.Store
	if cfg.PostgresDSN != "" {
		ctx := context.Background()
		var err error
		st, err = store.New(ctx, cfg.PostgresDSN)
		if err != nil {
			log.Fatalf("notification store: %v", err)
		}
		defer st.Close()
	} else {
		log.Println("notification: POSTGRES_DSN not set — inbox API disabled, email /send only")
	}

	tg := telegram.New(cfg.TelegramBotToken)
	alerts := telegram.New(cfg.TelegramAlertsToken)
	if alerts == nil {
		alerts = tg
	}
	h := handler.New(m, st, cfg.JWTSecret, tg, alerts, cfg.TelegramAlertsChatID)
	if cfg.RedisURL != "" {
		rdb, err := redis.New(cfg.RedisURL)
		if err != nil {
			log.Printf("notification: redis unavailable (%v)", err)
		} else {
			h.SetRedisPublisher(rdb)
			log.Println("notification: redis email queue enabled")
		}
	}
	if tg != nil {
		log.Println("notification: telegram user push enabled")
	} else {
		log.Println("notification: TELEGRAM_BOT_TOKEN not set — user telegram push disabled")
	}
	if alerts != nil && cfg.TelegramAlertsChatID != 0 {
		log.Printf("notification: ops alerts chat enabled (chat_id=%d)", cfg.TelegramAlertsChatID)
	} else {
		log.Println("notification: TELEGRAM_ALERTS_CHAT_ID not set — ops alerts disabled")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":     "ok",
			"service":    "notification",
			"smtp":       m.Mode(),
			"host":       cfg.SMTPHost,
			"inbox":      boolStr(st != nil),
			"telegram":   boolStr(tg != nil),
			"ops_alerts": boolStr(alerts != nil && cfg.TelegramAlertsChatID != 0),
		})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /send", h.Send)
	mux.HandleFunc("GET /notifications", h.ListNotifications)
	mux.HandleFunc("PATCH /notifications/{id}/read", h.MarkNotificationRead)
	mux.HandleFunc("POST /notifications/read-all", h.MarkAllNotificationsRead)
	mux.HandleFunc("POST /admin/notifications/send", h.AdminSendNotification)
	mux.HandleFunc("POST /system/notify", h.SystemNotify)
	mux.HandleFunc("POST /system/notify-telegram", h.NotifyTelegram)
	mux.HandleFunc("POST /system/ops-alert", h.OpsAlert)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: mux, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	log.Printf("notification on :%s | smtp %s:%s mode=%s from=%s inbox=%v telegram=%v ops_alerts=%v",
		cfg.Port, cfg.SMTPHost, cfg.SMTPPort, m.Mode(), cfg.From, st != nil, tg != nil, alerts != nil && cfg.TelegramAlertsChatID != 0)
	log.Fatal(srv.ListenAndServe())
}

func boolStr(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
