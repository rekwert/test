package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

func handlePasswordChange(ctx context.Context, st *store.Store, hv hypervisor.Adapter, payload json.RawMessage) error {
	var data struct {
		InstanceID string `json:"instance_id"`
		UserID     string `json:"user_id"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return err
	}
	instanceID := strings.TrimSpace(data.InstanceID)
	userID := strings.TrimSpace(data.UserID)
	if instanceID == "" {
		return fmt.Errorf("missing instance_id")
	}
	if userID == "" {
		if owner, err := st.GetInstanceOwner(ctx, instanceID); err == nil {
			userID = owner
		}
	}

	inst, err := st.GetInstanceForUser(ctx, userID, instanceID)
	if err != nil {
		return err
	}
	if store.IsDedicatedProvider(inst.Provider) {
		return fmt.Errorf("password change not supported for dedicated servers")
	}
	if inst.State == "creating" || inst.State == "queued" || inst.State == "reinstalling" || inst.State == "deleted" {
		return nil
	}
	if inst.IPAddress == nil || *inst.IPAddress == "" {
		return fmt.Errorf("server is still provisioning")
	}

	externalID, err := st.GetInstanceExternalID(ctx, instanceID)
	if err != nil || externalID == "" {
		return fmt.Errorf("server is still provisioning")
	}

	customPassword, err := st.TakePendingPasswordChange(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("read pending password: %w", err)
	}
	customPassword = strings.TrimSpace(customPassword)
	osTemplateID, _ := st.GetInstanceOSTemplateID(ctx, instanceID)
	isWindows := catalog.ResolveOSFamily(osTemplateID) == "windows"
	loginUser := catalog.ResolvePasswordResetUser(osTemplateID)
	ip := strings.TrimSpace(*inst.IPAddress)
	trustedKeys := trustedSSHHostKeys(ctx, st, ip)

	var newPassword string
	if customPassword != "" {
		currentPassword, err := st.GetInstanceRootPassword(ctx, instanceID)
		if err != nil {
			return fmt.Errorf("read root password: %w", err)
		}
		currentPassword = strings.TrimSpace(currentPassword)

		applied, applyErr := sshavail.ApplyDesiredPassword(ctx, ip, loginUser, currentPassword, customPassword, isWindows, trustedKeys...)
		if applyErr != nil && isWindows && currentPassword != "" {
			if vfPasswordResetAllowed(ctx, st, instanceID) {
				markVFPasswordReset(ctx, st, instanceID)
				vfPwd, vfErr := hv.ResetRootPassword(ctx, externalID, osTemplateID)
				if vfErr == nil {
					vfPwd = strings.TrimSpace(vfPwd)
					if vfPwd != "" {
						if applied2, applyErr2 := sshavail.ApplyDesiredPassword(ctx, ip, loginUser, vfPwd, customPassword, true, trustedKeys...); applyErr2 == nil {
							applied = applied2
							applyErr = nil
						} else {
							applyErr = applyErr2
						}
					}
				}
			}
		}
		if applyErr != nil {
			log.Printf("set custom password %s (%s): %v", instanceID, ip, applyErr)
			if sshavail.PasswordAuthDisabled(applyErr) {
				return fmt.Errorf("password authentication disabled on server")
			}
			return fmt.Errorf("password change failed: %w", applyErr)
		}
		newPassword = applied
	} else {
		if !vfPasswordResetAllowed(ctx, st, instanceID) {
			return fmt.Errorf("password change cooldown")
		}
		markVFPasswordReset(ctx, st, instanceID)
		pwd, err := hv.ResetRootPassword(ctx, externalID, osTemplateID)
		if err != nil {
			return fmt.Errorf("reset root password: %w", err)
		}
		newPassword = strings.TrimSpace(pwd)
		if newPassword == "" {
			return fmt.Errorf("empty password from hypervisor")
		}
	}

	if err := st.UpdateInstanceRootPassword(ctx, instanceID, newPassword); err != nil {
		return fmt.Errorf("save root password: %w", err)
	}
	return nil
}

func trustedSSHHostKeys(ctx context.Context, st *store.Store, ip string) []string {
	stored, err := st.GetIPSSHHostKeys(ctx, ip)
	if err != nil || stored == nil {
		return nil
	}
	var keys []string
	if pub := strings.TrimSpace(stored.Ed25519Public); pub != "" {
		keys = append(keys, pub)
	}
	if pub := strings.TrimSpace(stored.ECDSAPublic); pub != "" {
		keys = append(keys, pub)
	}
	return keys
}

func failVPSProvisionRefund(ctx context.Context, st *store.Store, instanceID, userID string) {
	if userID == "" {
		if owner, err := st.GetInstanceOwner(ctx, instanceID); err == nil {
			userID = owner
		}
	}
	if userID == "" {
		return
	}
	already, _ := st.HasVPSInstanceRefund(ctx, instanceID)
	if already {
		return
	}
	amount, err := st.GetInstanceChargeAmount(ctx, instanceID)
	if err != nil || amount <= 0 {
		return
	}
	if err := st.CreditBalance(ctx, userID, amount, fmt.Sprintf("Refund VPS provision failure %s", instanceID)); err != nil {
		log.Printf("vps provision refund %s: %v", instanceID, err)
		return
	}
	log.Printf("vps provision refund %s: credited %.0f to user %s", instanceID, amount, userID)
}

func failVPSOrphanRefund(ctx context.Context, st *store.Store, instanceID string) {
	owner, err := st.GetInstanceOwner(ctx, instanceID)
	if err != nil || owner == "" {
		return
	}
	already, _ := st.HasVPSInstanceRefund(ctx, instanceID)
	if already {
		return
	}
	amount, err := st.GetInstanceChargeAmount(ctx, instanceID)
	if err != nil || amount <= 0 {
		return
	}
	desc := fmt.Sprintf("Refund VPS hypervisor orphan %s", instanceID)
	if err := st.CreditBalance(ctx, owner, amount, desc); err != nil {
		log.Printf("vps orphan refund %s: %v", instanceID, err)
		return
	}
	log.Printf("vps orphan refund %s: credited %.0f to user %s", instanceID, amount, owner)
}

func outboxUserID(payload json.RawMessage) string {
	var data struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.UserID)
}
