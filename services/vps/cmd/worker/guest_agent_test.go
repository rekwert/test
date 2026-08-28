package main

import (
	"testing"
	"time"
)

func TestGuestAgentWarmupSameDurationForAllOS(t *testing.T) {
	t.Setenv("VPS_GUEST_AGENT_WARMUP", "50ms")
	t.Setenv("VPS_VF_RESET_MIN_GAP", "40ms")

	if guestAgentWarmupDuration() != 50*time.Millisecond {
		t.Fatalf("warmup = %s, want 50ms", guestAgentWarmupDuration())
	}
	if hvResetMinGap() != 40*time.Millisecond {
		t.Fatalf("reset gap = %s, want 40ms", hvResetMinGap())
	}

	instanceID := "test-instance-unified"
	clearGuestAgentWarmup(t.Context(), nil, instanceID)

	if guestAgentWarmupReady(t.Context(), nil, instanceID) {
		t.Fatal("expected warmup to block on first call without store")
	}
	guestAgentWarmupAt.Store(instanceID, time.Now().Add(-60*time.Millisecond))
	if !guestAgentWarmupReady(t.Context(), nil, instanceID) {
		t.Fatal("expected warmup ready after elapsed duration")
	}
}

func TestVFPasswordResetCooldownUnified(t *testing.T) {
	t.Setenv("VPS_VF_RESET_MIN_GAP", "100ms")
	instanceID := "inst-99"
	clearVFPasswordResetCache(t.Context(), nil, instanceID)

	if !vfPasswordResetAllowed(t.Context(), nil, instanceID) {
		t.Fatal("expected first reset allowed")
	}
	markVFPasswordReset(t.Context(), nil, instanceID)
	if vfPasswordResetAllowed(t.Context(), nil, instanceID) {
		t.Fatal("expected cooldown to block immediate second reset")
	}
	lastHVPasswordReset.Store(instanceID, time.Now().Add(-150*time.Millisecond))
	if !vfPasswordResetAllowed(t.Context(), nil, instanceID) {
		t.Fatal("expected reset allowed after cooldown")
	}
}
