package main

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

// Unified provision/reinstall timing for every OS template (admin issue, client order, reinstall).
const defaultGuestAgentWarmup = 45 * time.Second
const defaultHVResetMinGap = 30 * time.Second

var guestAgentWarmupAt sync.Map // instanceID -> time.Time
var lastHVPasswordReset sync.Map // instanceID -> time.Time

func guestAgentWarmupDuration() time.Duration {
	return envDuration("VPS_GUEST_AGENT_WARMUP", defaultGuestAgentWarmup)
}

func hvResetMinGap() time.Duration {
	return envDuration("VPS_HV_RESET_MIN_GAP", envDuration("VPS_VF_RESET_MIN_GAP", defaultHVResetMinGap))
}

func startGuestAgentWarmupClock(ctx context.Context, st *store.Store, instanceID string) {
	if st == nil || strings.TrimSpace(instanceID) == "" {
		return
	}
	if _, err := st.GuestAgentWarmupAt(ctx, instanceID); err != nil {
		log.Printf("guest-agent warmup start %s: %v", instanceID, err)
	}
}

// bootstrapPasswordWorks reports whether the user-chosen password already works over SSH.
// When true, VirtFusion resetPassword and the full guest-agent warmup can be skipped.
func bootstrapPasswordWorks(ctx context.Context, password, ip, osTemplateID string) bool {
	password = strings.TrimSpace(password)
	ip = strings.TrimSpace(ip)
	if password == "" || ip == "" || catalog.ResolveOSFamily(osTemplateID) == "windows" {
		return false
	}
	loginUser := catalog.ResolvePasswordResetUser(osTemplateID)
	if err := sshavail.CheckUserPassword(ctx, ip, loginUser, password); err == nil {
		return true
	}
	return sshavail.CheckRootPassword(ctx, ip, password) == nil
}

func guestAgentWarmupReady(ctx context.Context, st *store.Store, instanceID string) bool {
	if strings.TrimSpace(instanceID) == "" {
		return true
	}
	dur := guestAgentWarmupDuration()
	if v, ok := guestAgentWarmupAt.Load(instanceID); ok {
		return time.Since(v.(time.Time)) >= dur
	}
	var started time.Time
	if st == nil {
		started = time.Now()
	} else if t, err := st.GuestAgentWarmupAt(ctx, instanceID); err != nil {
		log.Printf("guest-agent warmup mark %s: %v", instanceID, err)
		started = time.Now()
	} else {
		started = t
	}
	guestAgentWarmupAt.Store(instanceID, started)
	return time.Since(started) >= dur
}

func clearGuestAgentWarmup(ctx context.Context, st *store.Store, instanceID string) {
	if instanceID != "" {
		guestAgentWarmupAt.Delete(instanceID)
		if st != nil {
			_ = st.ClearGuestAgentWarmupMeta(ctx, instanceID)
		}
	}
}

func vfPasswordResetAllowed(ctx context.Context, st *store.Store, instanceID string) bool {
	if strings.TrimSpace(instanceID) == "" {
		return true
	}
	gap := hvResetMinGap()
	if st != nil {
		if t, ok, err := st.LastVFPasswordResetAt(ctx, instanceID); err == nil && ok {
			lastHVPasswordReset.Store(instanceID, t)
			return time.Since(t) >= gap
		}
	}
	if v, ok := lastHVPasswordReset.Load(instanceID); ok {
		return time.Since(v.(time.Time)) >= gap
	}
	return true
}

func markVFPasswordReset(ctx context.Context, st *store.Store, instanceID string) {
	if strings.TrimSpace(instanceID) == "" {
		return
	}
	if st != nil {
		if t, err := st.VFPasswordResetAt(ctx, instanceID); err == nil {
			lastHVPasswordReset.Store(instanceID, t)
			return
		}
	}
	lastHVPasswordReset.Store(instanceID, time.Now())
}

func clearVFPasswordResetCache(ctx context.Context, st *store.Store, instanceID string) {
	if strings.TrimSpace(instanceID) == "" {
		return
	}
	lastHVPasswordReset.Delete(instanceID)
	if st != nil {
		_ = st.ClearVFPasswordResetMeta(ctx, instanceID)
	}
}

// ensureQemuGuestAgent installs and starts qemu-guest-agent over SSH so VirtFusion
// resetPassword queue can use guest-set-user-password (FI builds often leave agent disconnected).
func ensureQemuGuestAgent(ctx context.Context, ip, password, osTemplateID string) {
	if strings.TrimSpace(ip) == "" || catalog.ResolveOSFamily(osTemplateID) == "windows" {
		return
	}
	if hypervisor.MockEnabled() {
		return
	}
	script := `export DEBIAN_FRONTEND=noninteractive
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -qq >/dev/null 2>&1 || true
  apt-get install -y -qq qemu-guest-agent >/dev/null 2>&1 || true
fi
if command -v systemctl >/dev/null 2>&1; then
  systemctl enable qemu-guest-agent >/dev/null 2>&1 || true
  systemctl restart qemu-guest-agent >/dev/null 2>&1 || true
fi
`
	if err := sshavail.RunScript(ctx, ip, password, script); err != nil {
		log.Printf("ensure qemu-guest-agent %s (%s): %v", ip, osTemplateID, err)
		return
	}
	log.Printf("ensure qemu-guest-agent %s: installed/restarted", ip)
}
