package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/abuse"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/handler"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

func main() {
	port := env("PORT", "8003")
	jwtSecret := prodenv.RequireJWTSecret("dev-secret")
	prodenv.AssertPostgresDSNSecurity(os.Getenv("POSTGRES_DSN"))

	var st *store.Store
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		ctx := context.Background()
		var err error
		st, err = store.New(ctx, dsn)
		if err != nil {
			log.Fatalf("db: %v", err)
		}
		defer st.Close()
		if m, err := st.ListOSExternalMap(ctx); err == nil && len(m) > 0 {
			hypervisor.SetDynamicOSTemplates(m)
		}
	}

	h := handler.New(jwtSecret, st)
	hypervisor.AssertProductionHypervisor()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /ready", ready)

	mux.HandleFunc("GET /plans", h.ListPlans)
	mux.HandleFunc("GET /catalog", h.Catalog)
	mux.HandleFunc("GET /catalog/regions", h.ListRegions)
	mux.HandleFunc("GET /catalog/os", h.ListOSTemplates)
	mux.HandleFunc("GET /catalog/os/{os_id}/software", h.SoftwareForOS)

	mux.HandleFunc("POST /plans", stub("create plan"))
	mux.HandleFunc("POST /orders", h.CreateOrder)
	mux.HandleFunc("GET /free-week", h.FreeWeekStatus)
	mux.HandleFunc("GET /instance-slug/{slug}", h.GetInstanceBySlug)
	mux.HandleFunc("GET /instances", h.ListInstances)
	mux.HandleFunc("GET /instances/{id}", h.GetInstance)
	mux.HandleFunc("POST /instances/{id}/extend", h.InstanceExtend)
	mux.HandleFunc("GET /instances/{id}/upgrade-quote", h.InstanceUpgradeQuote)
	mux.HandleFunc("POST /instances/{id}/upgrade", h.InstanceUpgrade)
	mux.HandleFunc("POST /instances/{id}/change-ip", h.InstanceChangeIP)
	mux.HandleFunc("POST /instances/{id}/rent-ipv4", h.InstanceRentIPv4)
	mux.HandleFunc("PATCH /instances/{id}/auto-renew", h.InstanceSetAutoRenew)
	mux.HandleFunc("PATCH /instances/{id}/hostname", h.InstanceRename)
	mux.HandleFunc("GET /instances/{id}/credentials", h.InstanceCredentials)
	mux.HandleFunc("POST /instances/{id}/password", h.InstanceChangePassword)
	mux.HandleFunc("POST /instances/{id}/start", h.InstanceAction)
	mux.HandleFunc("POST /instances/{id}/stop", h.InstanceAction)
	mux.HandleFunc("POST /instances/{id}/reboot", h.InstanceAction)
	mux.HandleFunc("DELETE /instances/{id}", h.InstanceAction)
	mux.HandleFunc("GET /instances/{id}/console", h.InstanceConsole)
	mux.HandleFunc("GET /instances/{id}/console/ws", h.InstanceConsoleWS)
	mux.HandleFunc("POST /instances/{id}/console/ws-token", h.InstanceConsoleWSToken)
	mux.HandleFunc("GET /instances/{id}/metrics", h.InstanceMetrics)
	mux.HandleFunc("POST /instances/{id}/reinstall", h.InstanceReinstall)
	mux.HandleFunc("POST /instances/{id}/rescue", h.InstanceRescue)
	mux.HandleFunc("GET /instances/{id}/snapshots", h.ListInstanceSnapshots)
	mux.HandleFunc("POST /instances/{id}/snapshots", h.CreateInstanceSnapshot)
	mux.HandleFunc("DELETE /instances/{id}/snapshots/{snapshot_id}", h.DeleteInstanceSnapshot)
	mux.HandleFunc("GET /instances/{id}/backups", h.ListInstanceBackups)
	mux.HandleFunc("POST /instances/{id}/backups", h.CreateInstanceBackup)

	mux.HandleFunc("GET /admin/stats", h.AdminStats)
	mux.HandleFunc("GET /admin/instances", h.AdminListInstances)
	mux.HandleFunc("GET /admin/nodes", h.AdminListNodes)
	mux.HandleFunc("PATCH /admin/nodes/{id}/tiers", h.AdminUpdateNodeTiers)
	mux.HandleFunc("GET /admin/region-tiers", h.AdminListRegionTiers)
	mux.HandleFunc("PATCH /admin/region-tiers/{region}/{tier}", h.AdminUpdateRegionTier)
	mux.HandleFunc("GET /admin/clients/{user_id}/instances", h.AdminListUserInstances)
	mux.HandleFunc("POST /admin/clients/{user_id}/instances", h.AdminIssueInstance)
	mux.HandleFunc("GET /admin/clients/{user_id}/actions", h.AdminClientActions)
	mux.HandleFunc("GET /admin/clients/{user_id}/ip-history", h.AdminClientIPHistory)
	mux.HandleFunc("POST /admin/clients/{user_id}/suspend", h.AdminSuspendClient)
	mux.HandleFunc("POST /admin/clients/{user_id}/unsuspend", h.AdminUnsuspendClient)
	mux.HandleFunc("GET /admin/instances/{id}/upgrade-quote", h.AdminUpgradeQuote)
	mux.HandleFunc("POST /admin/instances/{id}/upgrade", h.AdminUpgradeInstance)
	mux.HandleFunc("PATCH /admin/instances/{id}/billing", h.AdminExtendInstanceBilling)
	mux.HandleFunc("GET /admin/instances/{id}/diagnostics", h.AdminInstanceDiagnostics)
	mux.HandleFunc("POST /admin/clients/{user_id}/transfer", h.AdminTransferClient)
	mux.HandleFunc("POST /admin/tools/subuser-check", h.AdminSubuserCheck)
	mux.HandleFunc("POST /admin/tools/ip-check", h.AdminIPCheck)
	mux.HandleFunc("GET /admin/tools/vm-by-ip", h.AdminVMByIP)
	mux.HandleFunc("POST /admin/tools/vm-block", h.AdminVMBlock)
	mux.HandleFunc("POST /admin/tools/vm-unblock", h.AdminVMUnblock)
	mux.HandleFunc("POST /admin/tools/instances/{id}/block", h.AdminInstanceBlock)
	mux.HandleFunc("POST /admin/tools/instances/{id}/unblock", h.AdminInstanceUnblock)
	mux.HandleFunc("POST /admin/tools/instances/{id}/smtp-outbound/open", h.AdminInstanceSMTPOpen)
	mux.HandleFunc("POST /admin/tools/instances/{id}/smtp-outbound/close", h.AdminInstanceSMTPClose)
	mux.HandleFunc("POST /admin/tools/instances/{id}/change-ip", h.AdminInstanceChangeIP)
	mux.HandleFunc("POST /admin/tools/instances/{id}/start", h.AdminInstanceAction)
	mux.HandleFunc("POST /admin/tools/instances/{id}/stop", h.AdminInstanceAction)
	mux.HandleFunc("POST /admin/tools/instances/{id}/reboot", h.AdminInstanceAction)
	mux.HandleFunc("POST /admin/tools/instances/{id}/delete", h.AdminInstanceAction)
	mux.HandleFunc("DELETE /admin/tools/instances/{id}", h.AdminInstanceAction)

	mux.HandleFunc("POST /internal/abuse/signal", h.InternalAbuseSignal)
	mux.HandleFunc("POST /admin/abuse/cases/{id}/false-positive", h.AdminAbuseFalsePositive)

	abuseCfg := abuse.LoadConfig()
	if abuseCfg.Enabled {
		ingestOK := strings.TrimSpace(os.Getenv("ABUSE_INGEST_TOKEN")) != "" ||
			strings.TrimSpace(os.Getenv("NOTIFICATION_SERVICE_TOKEN")) != "" ||
			strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN")) != ""
		if ingestOK {
			log.Printf("vps service: abuse auto-stop enabled (threshold=%d, ingest auth ok)", abuseCfg.AutoStopThreshold)
		} else {
			log.Printf("vps service: abuse auto-stop enabled but ABUSE_INGEST_TOKEN unset — POST /internal/abuse/signal will reject all requests")
		}
	} else {
		log.Printf("vps service: abuse auto-stop disabled (ABUSE_ENABLED=false)")
	}

	// Power / password / resize calls to VirtFusion can exceed 15s.
	// WriteTimeout=0: console/ws stays open for VNC sessions.
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadTimeout: 15 * time.Second, WriteTimeout: 0, IdleTimeout: 120 * time.Second}
	log.Printf("vps service listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}

func health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "vps"})
}

func ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func stub(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"endpoint": name, "status": "todo"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
