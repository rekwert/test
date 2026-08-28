package main

import (
	"context"
	"log"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
	"github.com/google/uuid"
)

func syncInstanceStates(ctx context.Context, st *store.Store, hv hypervisor.Adapter) error {
	if st == nil || hv == nil {
		return nil
	}
	items, err := st.ListInstancesForHypervisorSync(ctx, 30)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !looksLikeOpenStackInstanceID(item.ExternalID) {
			log.Printf("instance sync: invalid external_id %s for %s", item.ExternalID, item.ID)
			if ok, err := st.MarkInstanceOrphaned(ctx, item.ID, "invalid openstack instance id"); err != nil {
				log.Printf("instance sync orphan %s: %v", item.ID, err)
			} else if ok {
				failVPSOrphanRefund(ctx, st, item.ID)
			}
			continue
		}
		server, err := hv.GetServer(ctx, item.ExternalID)
		if err != nil {
			if hypervisor.IsServerNotFound(err) {
				log.Printf("instance sync: openstack server missing for %s (os=%s)", item.ID, item.ExternalID)
				missCount, incErr := st.IncrementInstanceSyncMiss(ctx, item.ID)
				if incErr != nil {
					log.Printf("instance sync miss count %s: %v", item.ID, incErr)
					continue
				}
				if missCount >= 3 {
					if ok, err := st.MarkInstanceOrphaned(ctx, item.ID, "openstack server missing"); err != nil {
						log.Printf("instance sync orphan %s: %v", item.ID, err)
					} else if ok {
						failVPSOrphanRefund(ctx, st, item.ID)
					}
				}
			}
			continue
		}
		_ = st.ClearInstanceSyncMiss(ctx, item.ID)
		state := hypervisor.MapServerState(server.Status)
		if store.ClientPowerBlocked(item.BillingStatus) && hypervisor.ServerPoweredOn(server) {
			if err := enforceBillingPowerOff(ctx, hv, item.ExternalID); err != nil {
				log.Printf("instance sync billing poweroff %s (os=%s): %v", item.ID, item.ExternalID, err)
			} else if !billingPowerOffApplied(ctx, hv, item.ExternalID) {
				log.Printf("instance sync billing poweroff %s (os=%s): guest still running after poweroff+suspend", item.ID, item.ExternalID)
			} else {
				log.Printf("instance sync billing poweroff %s (os=%s)", item.ID, item.ExternalID)
				_ = st.MarkInstanceStopped(ctx, item.ID)
			}
			continue
		}
		if item.State == "error" || item.HasProvisionError {
			if item.HasProvisionError && item.State == "running" && server.IP != "" && hypervisor.ServerPoweredOn(server) {
				if err := st.ClearInstanceProvisionError(ctx, item.ID); err != nil {
					log.Printf("instance sync heal provision_error %s: %v", item.ID, err)
				} else {
					log.Printf("instance sync cleared stale provision_error %s", item.ID)
				}
			}
			continue
		}
		ip := server.IP
		if err := st.UpdateInstanceFromHypervisor(ctx, item.ID, state, ip); err != nil {
			log.Printf("instance sync apply %s: %v", item.ID, err)
		}
	}
	return nil
}

func looksLikeOpenStackInstanceID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	_, err := uuid.Parse(id)
	return err == nil
}
