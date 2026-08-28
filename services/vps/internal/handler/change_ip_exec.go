package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/smtpblock"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

type changeIPExecError struct {
	status int
	msg    string
}

func (e changeIPExecError) Error() string { return e.msg }

type changeIPResult struct {
	OldIP         string
	NewIP         string
	AmountCharged float64
	GuestUpdated  bool
	Instance      *store.Instance
}

func (h *Handler) executeInstanceChangeIP(ctx context.Context, userID, instanceID, oldIP, actorID string, waiveFee bool) (*changeIPResult, error) {
	// Survive browser close / AbortSignal / gateway timeout: finish VF+SSH work,
	// refunds and lock cleanup even if the HTTP request is canceled.
	const changeIPWorkTimeout = 4 * time.Minute
	workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), changeIPWorkTimeout)
	defer cancel()

	inst, err := h.store.GetInstanceForUser(workCtx, userID, instanceID)
	if err != nil {
		return nil, changeIPExecError{status: http.StatusNotFound, msg: "instance not found"}
	}
	if inst.State == "creating" || inst.State == "queued" || inst.State == "reinstalling" || inst.State == "deleted" {
		return nil, changeIPExecError{status: http.StatusConflict, msg: "server is not ready"}
	}
	if inst.IPAddress == nil || *inst.IPAddress == "" {
		return nil, changeIPExecError{status: http.StatusConflict, msg: "server has no ip yet"}
	}
	provider := strings.TrimSpace(strings.ToLower(inst.Provider))
	if provider != "" && provider != "openstack" {
		return nil, changeIPExecError{status: http.StatusBadRequest, msg: "ip change not supported for this server type"}
	}
	externalID, err := h.store.GetInstanceExternalID(workCtx, instanceID)
	if err != nil || externalID == "" {
		return nil, changeIPExecError{status: http.StatusConflict, msg: "server is still provisioning"}
	}

	primaryIP := strings.TrimSpace(*inst.IPAddress)
	requestedOldIP := strings.TrimSpace(oldIP)
	if requestedOldIP == "" {
		oldIP = primaryIP
	} else {
		oldIP = requestedOldIP
	}
	isPrimary := requestedOldIP == "" || requestedOldIP == primaryIP

	creds, credErr := h.store.GetInstanceCredentials(workCtx, userID, instanceID)
	rootPass := ""
	var credAllIPs []string
	if credErr == nil {
		rootPass = strings.TrimSpace(creds.RootPassword)
		credAllIPs = creds.AllIPs
	}

	hv := h.hv()
	attachedIPs, gateway, netmask, ipInfoErr := hv.PrimaryIPv4Info(workCtx, externalID)
	if ipInfoErr != nil || len(attachedIPs) == 0 {
		if len(credAllIPs) > 0 {
			attachedIPs = credAllIPs
		} else {
			attachedIPs = []string{oldIP}
		}
	}
	if !ipOnServer(attachedIPs, oldIP) {
		return nil, changeIPExecError{status: http.StatusBadRequest, msg: "invalid ip"}
	}

	prefix := sshavail.PrefixFromNetmask(netmask)
	reachCandidates := guestReachCandidates(oldIP, attachedIPs, credAllIPs)

	osTemplateID, _ := h.store.GetInstanceOSTemplateID(workCtx, instanceID)
	guestAuth := guestSSHAuth(rootPass)
	if isPrimary {
		var bootPass string
		bootPass, guestAuth, _ = h.bootstrapGuestSSHForChangeIP(workCtx, instanceID, externalID, osTemplateID, reachCandidates, rootPass)
		if bootPass != "" {
			rootPass = bootPass
		}
		reachIP, err := sshavail.DialAnyRootAuth(workCtx, reachCandidates, guestAuth)
		if err != nil {
			log.Printf("change ip guest ssh unavailable %s: %v", instanceID, err)
			return nil, changeIPExecError{
				status: http.StatusBadGateway,
				msg:    "ip change failed: guest ssh unavailable (keys-only access)",
			}
		}
		// Self-heal partial failures: drop VF orphans the guest is not using and
		// reconfigure from the address that actually responds.
		if reachIP != oldIP || len(attachedIPs) > 1 {
			if len(attachedIPs) > 1 {
				releaseExtraPrimaryIPv4(workCtx, hv, externalID, reachIP, attachedIPs)
				if synced, gw, mask, syncErr := hv.PrimaryIPv4Info(workCtx, externalID); syncErr == nil && len(synced) > 0 {
					attachedIPs = synced
					if gw != "" {
						gateway = gw
					}
					if mask != "" {
						prefix = sshavail.PrefixFromNetmask(mask)
					}
				}
			}
			if reachIP != oldIP {
				log.Printf("change ip %s guest reachable on %s (portal/vf old %s)", instanceID, reachIP, oldIP)
				oldIP = reachIP
			}
			reachCandidates = guestReachCandidates(oldIP, attachedIPs, credAllIPs)
		}
	}

	locked, err := h.store.TryBeginChangeIP(workCtx, userID, instanceID)
	if err != nil {
		return nil, changeIPExecError{status: http.StatusInternalServerError, msg: "change ip failed"}
	}
	if !locked {
		return nil, changeIPExecError{status: http.StatusConflict, msg: "ip change already in progress"}
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := h.store.ClearChangeIPLock(cleanupCtx, instanceID); err != nil {
			log.Printf("change ip clear lock %s: %v", instanceID, err)
		}
	}()

	amountCharged := 0.0
	if !waiveFee {
		if err := h.store.ChargeChangeIPFee(workCtx, userID, instanceID); err != nil {
			switch {
			case errors.Is(err, store.ErrInsufficientBalance):
				return nil, changeIPExecError{status: http.StatusPaymentRequired, msg: "insufficient balance"}
			case errors.Is(err, store.ErrBillingSuspended):
				return nil, changeIPExecError{status: http.StatusForbidden, msg: "billing account suspended"}
			default:
				return nil, changeIPExecError{status: http.StatusInternalServerError, msg: "charge failed"}
			}
		}
		amountCharged = store.ChangeIPFee
	}

	changeOperationID := time.Now().UTC().Format("20060102T150405.000000000Z")
	refund := func(reason string) {
		if waiveFee {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		refundReason := reason + " [instance=" + instanceID + " operation=" + changeOperationID + "]"
		if err := h.store.RefundChangeIPFee(cleanupCtx, userID, instanceID, refundReason); err != nil {
			log.Printf("change ip refund %s: %v", instanceID, err)
		}
	}
	removeAssignedIP := func(ip string) {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := hv.RemovePrimaryIPv4(cleanupCtx, externalID, ip); err != nil {
			log.Printf("change ip rollback new %s (%s): %v", instanceID, ip, err)
		}
	}

	newIP, err := hv.AddPrimaryIPv4(workCtx, externalID)
	if err != nil {
		log.Printf("change ip assign %s: %v", instanceID, err)
		refund("VPS IP change refund (assign failed)")
		msg := "ip change failed"
		if isNoFreeIPError(err) {
			msg = "no free ip addresses available"
		}
		return nil, changeIPExecError{status: http.StatusBadGateway, msg: msg}
	}

	if err := hv.SyncServerNetworkFilters(workCtx, externalID); err != nil {
		log.Printf("change ip network filter sync after assign %s: %v", instanceID, err)
		removeAssignedIP(newIP)
		refund("VPS IP change refund (hypervisor network sync failed)")
		return nil, changeIPExecError{status: http.StatusBadGateway, msg: "ip change failed: hypervisor network sync failed"}
	}

	guestOK := !isPrimary
	if isPrimary {
		_ = sleepCtx(workCtx, 3*time.Second)
		guestOK = reconfigurePrimaryIPGuest(workCtx, instanceID, reachCandidates, newIP, gateway, prefix, guestAuth)
	}

	if isPrimary && !guestOK {
		if err := hv.RebootServer(workCtx, externalID); err != nil {
			log.Printf("change ip reboot fallback %s: %v", instanceID, err)
		} else {
			_ = sleepCtx(workCtx, 35*time.Second)
			guestOK = reconfigurePrimaryIPGuest(workCtx, instanceID, reachCandidates, newIP, gateway, prefix, guestAuth)
		}
	}

	if isPrimary && !guestOK {
		removeAssignedIP(newIP)
		refund("VPS IP change refund (guest reconfigure failed)")
		return nil, changeIPExecError{status: http.StatusBadGateway, msg: "ip change failed: guest network was not updated"}
	}

	// Once the guest is reachable on the new address, keep each final step on
	// its own context. A slow VF filter refresh must never cancel the DB save.
	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 45*time.Second)
	if attached, _, _, err := hv.PrimaryIPv4Info(releaseCtx, externalID); err == nil && len(attached) > 0 {
		releaseExtraPrimaryIPv4(releaseCtx, hv, externalID, newIP, attached)
	} else {
		if err := hv.RemovePrimaryIPv4(releaseCtx, externalID, oldIP); err != nil {
			log.Printf("change ip release old %s (%s): %v", instanceID, oldIP, err)
		}
	}
	releaseCancel()

	ipLogOpts := &store.IPAssignmentLogOpts{Source: store.IPSourceChangeIP, ActorID: actorID}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer saveCancel()
	if isPrimary {
		if err := h.store.SetInstanceIPAddress(saveCtx, instanceID, newIP, ipLogOpts); err != nil {
			log.Printf("change ip save %s: %v", instanceID, err)
			refund("VPS IP change refund (save failed)")
			return nil, changeIPExecError{status: http.StatusInternalServerError, msg: "ip changed but save failed"}
		}
	} else {
		_ = h.store.LogIPChange(saveCtx, userID, instanceID, oldIP, newIP, ipLogOpts)
	}
	if ips, _, _, err := hv.PrimaryIPv4Info(saveCtx, externalID); err == nil && len(ips) > 0 {
		_ = h.store.SetInstanceAllIPs(saveCtx, instanceID, ips)
	}

	inst, err = h.store.GetInstanceForUser(saveCtx, userID, instanceID)
	if err != nil {
		inst = nil
	}

	if isPrimary && inst != nil && inst.SmtpOutboundOpen {
		go func(id, newAddr string) {
			syncCtx, syncCancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer syncCancel()
			target, tErr := h.store.GetSMTPControlTarget(syncCtx, id)
			if tErr != nil || target.HVHost == "" || newAddr == "" {
				return
			}
			if oldIP != "" {
				_ = smtpblock.SetGuestAllowed(syncCtx, target.HVHost, oldIP, false)
			}
			if err := smtpblock.SetGuestAllowed(syncCtx, target.HVHost, newAddr, true); err != nil {
				log.Printf("change ip smtp allowlist %s: %v", id, err)
			}
		}(instanceID, newIP)
	}

	// The assign-time sync already admitted the replacement IP. Releasing the
	// old-IP filter is best-effort cleanup and must not delay the API response.
	go func(id string) {
		syncCtx, syncCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer syncCancel()
		if err := hv.SyncServerNetworkFilters(syncCtx, externalID); err != nil {
			log.Printf("change ip network filter sync after release %s: %v", id, err)
		}
	}(instanceID)

	return &changeIPResult{
		OldIP:         oldIP,
		NewIP:         newIP,
		AmountCharged: amountCharged,
		GuestUpdated:  guestOK,
		Instance:      inst,
	}, nil
}

func writeChangeIPExecError(w http.ResponseWriter, err error) {
	var execErr changeIPExecError
	if errors.As(err, &execErr) {
		writeError(w, execErr.status, execErr.msg)
		return
	}
	writeError(w, http.StatusInternalServerError, "change ip failed")
}
