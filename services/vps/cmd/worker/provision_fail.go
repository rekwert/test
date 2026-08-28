package main

import (
	"context"
	"log"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/notify"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

// finalizeFailedProvision marks a creating instance as error, refunds the order charge,
// cleans up VirtFusion, and notifies the customer. Idempotent via FailProvisioningIfCreating.
func finalizeFailedProvision(ctx context.Context, st *store.Store, hv hypervisor.Adapter, instanceID, userID, hostname, errMsg string, kind notify.FailKind) bool {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return false
	}
	ok, failErr := st.FailProvisioningIfCreating(ctx, instanceID, errMsg)
	if failErr != nil {
		log.Printf("fail provision %s: %v", instanceID, failErr)
		return false
	}
	if !ok {
		return false
	}
	cleanupHypervisorOnProvisionFailure(ctx, st, hv, instanceID)
	failVPSProvisionRefund(ctx, st, instanceID, userID)
	_ = st.MarkProvisionOutboxPublished(ctx, instanceID)
	clearGuestAgentWarmup(ctx, st, instanceID)
	host := strings.TrimSpace(hostname)
	if host == "" {
		host = instanceID
	}
	if userID != "" {
		if notifyErr := notify.InstanceFailedEmail(ctx, userID, host, errMsg, kind); notifyErr != nil {
			log.Printf("fail notify %s: %v", instanceID, notifyErr)
		}
	}
	log.Printf("provision failed %s: %s", instanceID, errMsg)
	return true
}

func finalizeFailedCreating(ctx context.Context, st *store.Store, hv hypervisor.Adapter, item store.CreatingInstance, errMsg string, kind notify.FailKind) bool {
	host := item.Hostname
	return finalizeFailedProvision(ctx, st, hv, item.ID, item.UserID, host, errMsg, kind)
}
