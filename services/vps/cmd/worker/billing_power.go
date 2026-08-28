package main

import (
	"context"
	"log"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
)

// enforceBillingPowerOff powers off the VM and suspends it in VirtFusion when billing blocks access.
func enforceBillingPowerOff(ctx context.Context, hv hypervisor.Adapter, externalID string) error {
	if err := hv.PowerOffServer(ctx, externalID); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}
	if after, err := hv.GetServer(ctx, externalID); err == nil && !hypervisor.ServerPoweredOn(after) {
		return nil
	}
	if err := hv.SuspendServer(ctx, externalID); err != nil {
		log.Printf("billing suspend vf=%s after poweroff: %v", externalID, err)
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}
	return nil
}

func billingPowerOffApplied(ctx context.Context, hv hypervisor.Adapter, externalID string) bool {
	server, err := hv.GetServer(ctx, externalID)
	if err != nil {
		return false
	}
	return !hypervisor.ServerPoweredOn(server)
}
