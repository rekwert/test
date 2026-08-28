package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/config"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/store"
)

func main() {
	cfg := config.Load()
	prodenv.AssertBillingNotMock(cfg.Mock)
	prodenv.AssertPostgresDSNSecurity(cfg.PostgresDSN)
	if cfg.Mock {
		log.Println("billing worker: BILLING_MOCK=true, skipping charge processor")
		select {}
	}
	if cfg.PostgresDSN == "" {
		log.Fatal("POSTGRES_DSN is required for billing worker")
	}

	ctx := context.Background()
	st, err := store.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("billing worker store: %v", err)
	}
	defer st.Close()

	chargeInterval := envDuration("BILLING_CHARGE_INTERVAL", 15*time.Minute)
	dunningInterval := envDuration("BILLING_DUNNING_INTERVAL", 15*time.Minute)
	referralInterval := envDuration("BILLING_REFERRAL_INTERVAL", 15*time.Minute)
	dunningCfg := store.DunningConfig{
		GraceHours: envInt("DUNNING_GRACE_HOURS", 12),
		DeleteDays: envInt("DUNNING_DELETE_DAYS", 3),
	}

	log.Printf("billing worker started (charge=%s dunning=%s referral=%s grace=%dh delete=%dd)",
		chargeInterval, dunningInterval, referralInterval, dunningCfg.GraceHours, dunningCfg.DeleteDays)

	runCharges := func() {
		result, err := st.ProcessDueCharges(ctx)
		if err != nil {
			log.Printf("billing worker charges: %v", err)
			return
		}
		if result.Processed > 0 {
			log.Printf("billing worker: processed=%d charged=%d suspended=%d past_due=%d",
				result.Processed, result.Charged, result.Suspended, result.Failed)
		}
	}

	runDunning := func() {
		result, err := st.ProcessDunning(ctx, dunningCfg)
		if err != nil {
			log.Printf("billing worker dunning: %v", err)
			return
		}
		if result.GraceExpired > 0 || result.Deleted > 0 || result.Reminders > 0 || result.ReconciledStops > 0 {
			log.Printf("billing dunning: grace_expired=%d deleted=%d reminders=%d reconciled_stops=%d",
				result.GraceExpired, result.Deleted, result.Reminders, result.ReconciledStops)
		}
	}

	runReferralSettle := func() {
		n, err := st.SettleDueReferralEarnings(ctx, 200)
		if err != nil {
			log.Printf("billing worker referral settle: %v", err)
			return
		}
		if n > 0 {
			log.Printf("billing worker referral settle: credited=%d", n)
		}
	}

	runCharges()
	runDunning()
	runReferralSettle()
	// Lease/balance reminders: daily in the last 7 days before next_billing_at
	// (inbox + email + Telegram if linked). Entry still named ProcessFreeWeekReminders.
	runFreeWeekReminders := func() {
		n, err := st.ProcessFreeWeekReminders(ctx)
		if err != nil {
			log.Printf("billing worker lease reminders: %v", err)
			return
		}
		if n > 0 {
			log.Printf("billing worker lease reminders: %d", n)
		}
	}
	runFreeWeekReminders()

	chargeTicker := time.NewTicker(chargeInterval)
	dunningTicker := time.NewTicker(dunningInterval)
	referralTicker := time.NewTicker(referralInterval)
	defer chargeTicker.Stop()
	defer dunningTicker.Stop()
	defer referralTicker.Stop()

	for {
		select {
		case <-chargeTicker.C:
			runCharges()
			runFreeWeekReminders()
		case <-dunningTicker.C:
			runDunning()
		case <-referralTicker.C:
			runReferralSettle()
		}
	}
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
