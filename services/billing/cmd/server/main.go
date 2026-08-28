package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/borishru-boop/testVPStrade/services/billing/internal/config"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/handler"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/store"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

func main() {
	cfg := config.Load()
	prodenv.AssertBillingNotMock(cfg.Mock)
	prodenv.AssertPostgresDSNSecurity(cfg.PostgresDSN)
	mux := http.NewServeMux()

	if cfg.Mock {
		mock := handler.NewMock(cfg.JWTSecret)
		mux.HandleFunc("GET /health", mock.Health)
		mux.HandleFunc("GET /ready", mock.Ready)
		mux.HandleFunc("GET /balance", mock.Balance)
		mux.HandleFunc("GET /payment-methods", mock.PaymentMethods)
		mux.HandleFunc("POST /topup", mock.Topup)
		mux.HandleFunc("POST /promo/validate", mock.ValidatePromo)
		mux.HandleFunc("POST /promo/apply", mock.ApplyPromo)
		mux.HandleFunc("GET /invoices", mock.Invoices)
		mux.HandleFunc("POST /payment", mock.PaymentWebhook)
		mux.HandleFunc("POST /heleket", mock.HeleketWebhook)
		mux.HandleFunc("POST /robokassa", mock.RobokassaWebhook)
		log.Printf("billing service listening on :%s (mock=true)", cfg.Port)
	} else {
		if cfg.PostgresDSN == "" {
			log.Fatal("POSTGRES_DSN is required when BILLING_MOCK=false")
		}
		ctx := context.Background()
		st, err := store.New(ctx, cfg.PostgresDSN)
		if err != nil {
			log.Fatalf("billing store: %v", err)
		}
		defer st.Close()

		h := handler.New(cfg, st, cfg.JWTSecret)
		mux.HandleFunc("GET /health", h.Health)
		mux.HandleFunc("GET /ready", h.Ready)
		mux.HandleFunc("GET /balance", h.Balance)
		mux.HandleFunc("GET /payment-methods", h.PaymentMethods)
		mux.HandleFunc("POST /topup", h.Topup)
		mux.HandleFunc("POST /promo/validate", h.ValidatePromo)
		mux.HandleFunc("POST /promo/apply", h.ApplyPromo)
		mux.HandleFunc("GET /invoices", h.Invoices)
		mux.HandleFunc("GET /admin/users/{id}/balance", h.AdminUserBalance)
		mux.HandleFunc("GET /admin/users/{id}/invoices", h.AdminUserInvoices)
		mux.HandleFunc("GET /admin/users/{id}/adjustments", h.AdminAdjustments)
		mux.HandleFunc("GET /admin/users/{id}/ledger", h.AdminLedger)
		mux.HandleFunc("POST /admin/users/{id}/refund", h.AdminRefund)
		mux.HandleFunc("POST /admin/users/{id}/credit", h.AdminCredit)
		mux.HandleFunc("GET /admin/stats/business", h.AdminBusinessStats)
		mux.HandleFunc("GET /admin/topups", h.AdminListTopups)
		mux.HandleFunc("POST /payment", h.PaymentWebhook)
		mux.HandleFunc("POST /heleket", h.HeleketWebhook)
		mux.HandleFunc("POST /robokassa", h.RobokassaWebhook)
		mux.HandleFunc("GET /robokassa", h.RobokassaWebhook)
		log.Printf("billing service listening on :%s (mock=false tbank=%v robokassa=%v)", cfg.Port, cfg.TBankReady(), cfg.RobokassaReady())
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
