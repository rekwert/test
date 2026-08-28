package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

var (
	workerIdentityOnce sync.Once
	workerIdentity     string
)

func workerID() string {
	workerIdentityOnce.Do(func() {
		workerIdentity = strings.TrimSpace(os.Getenv("VPS_WORKER_ID"))
		if workerIdentity == "" {
			if h, err := os.Hostname(); err == nil {
				workerIdentity = strings.TrimSpace(h)
			}
		}
		if workerIdentity == "" {
			workerIdentity = "worker"
		}
		workerIdentity += "-" + strconv.Itoa(os.Getpid())
	})
	return workerIdentity
}

// ensureHypervisorServerAllocated returns OpenStack instance id for a creating instance.
// When another worker is already allocating, ok=false and err=nil so callers can retry later.
func ensureHypervisorServerAllocated(ctx context.Context, st *store.Store, hv hypervisor.Adapter, instanceID string, opts hypervisor.CreateOptions) (externalID string, ok bool, err error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return "", false, nil
	}
	externalID, err = st.GetInstanceExternalID(ctx, instanceID)
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(externalID) != "" {
		return externalID, true, nil
	}
	claimed, err := st.TryClaimProvisionAllocate(ctx, instanceID, workerID())
	if err != nil {
		return "", false, err
	}
	if !claimed {
		externalID, err = st.GetInstanceExternalID(ctx, instanceID)
		if err != nil {
			return "", false, err
		}
		if strings.TrimSpace(externalID) != "" {
			return externalID, true, nil
		}
		return "", false, nil
	}
	defer func() {
		if relErr := st.ReleaseProvisionAllocateClaim(ctx, instanceID); relErr != nil {
			log.Printf("release provision allocate claim %s: %v", instanceID, relErr)
		}
	}()

	externalID, err = st.GetInstanceExternalID(ctx, instanceID)
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(externalID) != "" {
		return externalID, true, nil
	}

	allocated, err := hv.AllocateServer(ctx, opts)
	if err != nil {
		return "", false, err
	}
	won, err := st.SetInstanceExternalIDIfEmpty(ctx, instanceID, allocated.ID)
	if err != nil {
		if delErr := hv.DeleteServer(ctx, allocated.ID); delErr != nil {
			log.Printf("cleanup orphan vf=%s for %s: %v", allocated.ID, instanceID, delErr)
		}
		return "", false, err
	}
	if !won {
		if delErr := hv.DeleteServer(ctx, allocated.ID); delErr != nil {
			log.Printf("cleanup duplicate vf=%s for %s: %v", allocated.ID, instanceID, delErr)
		}
		externalID, err = st.GetInstanceExternalID(ctx, instanceID)
		if err != nil {
			return "", false, err
		}
		return externalID, strings.TrimSpace(externalID) != "", nil
	}
	return allocated.ID, true, nil
}

// saveExternalIDOrCleanup persists VF server id; on DB failure deletes the orphan VF server (C-01).
func saveExternalIDOrCleanup(ctx context.Context, st *store.Store, hv hypervisor.Adapter, instanceID, vfID string) error {
	won, err := st.SetInstanceExternalIDIfEmpty(ctx, instanceID, vfID)
	if err != nil {
		log.Printf("save external_id %s vf=%s: %v; cleaning up orphan", instanceID, vfID, err)
		if delErr := hv.DeleteServer(ctx, vfID); delErr != nil {
			log.Printf("cleanup orphan vf=%s for %s: %v", vfID, instanceID, delErr)
		}
		return err
	}
	if !won {
		if delErr := hv.DeleteServer(ctx, vfID); delErr != nil {
			log.Printf("cleanup duplicate vf=%s for %s: %v", vfID, instanceID, delErr)
		}
	}
	return nil
}

// passwordSyncPhaseStart is when VF password sync may begin (after guest-agent warmup).
func passwordSyncPhaseStart(ctx context.Context, st *store.Store, instanceID string) time.Time {
	if st == nil || instanceID == "" {
		return time.Time{}
	}
	started, err := st.GuestAgentWarmupAt(ctx, instanceID)
	if err != nil || started.IsZero() {
		return time.Time{}
	}
	return started.Add(guestAgentWarmupDuration())
}

func outboxTerminalForProvision(ctx context.Context, st *store.Store, instanceID string) (bool, error) {
	state, err := st.GetInstanceState(ctx, instanceID)
	if err != nil {
		return false, err
	}
	switch state {
	case "creating", "reinstalling":
		return false, nil
	default:
		return true, nil
	}
}

// cleanupHypervisorOnProvisionFailure deletes a partial OpenStack server when portal provisioning is aborted.
func cleanupHypervisorOnProvisionFailure(ctx context.Context, st *store.Store, hv hypervisor.Adapter, instanceID string) {
	if st == nil || hv == nil || instanceID == "" {
		return
	}
	externalID, err := st.GetInstanceExternalID(ctx, instanceID)
	if err != nil || strings.TrimSpace(externalID) == "" {
		return
	}
	if err := hv.DeleteServer(ctx, externalID); err != nil {
		log.Printf("cleanup os=%s after provision fail %s: %v", externalID, instanceID, err)
		return
	}
	if err := st.ClearInstanceExternalID(ctx, instanceID); err != nil {
		log.Printf("clear external_id after hypervisor cleanup %s: %v", instanceID, err)
	} else {
		log.Printf("cleaned up os=%s for failed provision %s", externalID, instanceID)
	}
}

// requeueForIPPool returns instance to queued when Neutron has no free IPv4; frees partial server.
func requeueForIPPool(ctx context.Context, st *store.Store, hv hypervisor.Adapter, instanceID, externalID string) error {
	if externalID != "" {
		if err := hv.DeleteServer(ctx, externalID); err != nil {
			log.Printf("requeue ip pool cleanup os=%s instance=%s: %v", externalID, instanceID, err)
		}
		_ = st.ClearInstanceExternalID(ctx, instanceID)
	}
	ok, err := st.RequeueInstanceForIPPool(ctx, instanceID)
	if err != nil {
		return err
	}
	if ok {
		_ = st.MarkProvisionOutboxPublished(ctx, instanceID)
		log.Printf("requeued %s for ip pool (waiting for free ipv4)", instanceID)
	}
	return nil
}
