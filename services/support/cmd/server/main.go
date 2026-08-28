package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/services/support/internal/config"
	"github.com/borishru-boop/testVPStrade/services/support/internal/handler"
	"github.com/borishru-boop/testVPStrade/services/support/internal/store"
)

func main() {
	cfg := config.Load()
	prodenv.AssertPostgresDSNSecurity(cfg.PostgresDSN)
	if cfg.PostgresDSN == "" {
		log.Fatal("POSTGRES_DSN is required")
	}

	ctx := context.Background()
	st, err := store.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("support store: %v", err)
	}
	defer st.Close()

	h := handler.New(st, cfg.JWTSecret, cfg.ServiceToken, cfg.AttachmentsDir, cfg.MaxAttachmentBytes)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /ready", h.Ready)
	mux.HandleFunc("POST /tickets", h.CreateTicket)
	mux.HandleFunc("GET /tickets", h.ListTickets)
	mux.HandleFunc("GET /tickets/{id}", h.GetTicket)
	mux.HandleFunc("POST /tickets/{id}/messages", h.AddMessage)
	mux.HandleFunc("POST /tickets/{id}/close", h.CloseTicket)
	mux.HandleFunc("POST /tickets/{id}/attachments", h.UploadAttachment)
	mux.HandleFunc("GET /tickets/{id}/attachments/{attachmentId}", h.DownloadAttachment)
	mux.HandleFunc("POST /internal/tickets/guest", h.CreateGuestTicket)
	mux.HandleFunc("POST /internal/tickets/guest/messages", h.AddGuestMessage)
	mux.HandleFunc("GET /internal/tickets/guest", h.LookupGuestTicket)
	mux.HandleFunc("GET /admin/tickets", h.AdminListTickets)
	mux.HandleFunc("POST /admin/tickets/claim-next", h.AdminClaimNext)
	mux.HandleFunc("POST /admin/tickets/{id}/take", h.AdminTake)
	mux.HandleFunc("POST /admin/tickets/{id}/release", h.AdminRelease)
	mux.HandleFunc("PATCH /admin/tickets/{id}/messages/{messageId}", h.AdminUpdateMessage)
	mux.HandleFunc("DELETE /admin/tickets/{id}/messages/{messageId}", h.AdminDeleteMessage)
	mux.HandleFunc("PATCH /admin/tickets/{id}", h.AdminUpdate)
	mux.HandleFunc("POST /admin/tickets/{id}/reply-close", h.AdminReplyClose)
	mux.HandleFunc("GET /admin/shift", h.AdminShiftStatus)
	mux.HandleFunc("POST /admin/shift/start", h.AdminShiftStart)
	mux.HandleFunc("POST /admin/shift/end", h.AdminShiftEnd)
	mux.HandleFunc("GET /admin/workspace", h.AdminWorkspace)
	mux.HandleFunc("GET /admin/queue/config", h.AdminSlotConfig)
	mux.HandleFunc("PATCH /admin/queue/config", h.AdminSlotConfig)

	go func() {
		runExpire := func() {
			if err := st.ProcessExpiredTickets(context.Background()); err != nil {
				log.Printf("expire tickets: %v", err)
			}
		}
		runExpire()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			runExpire()
		}
	}()

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	log.Printf("support service listening on :%s", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}
