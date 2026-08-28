package handler

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/opsssh"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
)

func guestSSHAuth(rootPass string) sshavail.GuestSSHAuth {
	auth := sshavail.GuestSSHAuth{Password: strings.TrimSpace(rootPass)}
	if signer, ok := opsssh.Signer(); ok {
		auth.Signer = signer
	}
	return auth
}

// guestReachCandidates merges portal/VF/credential IPs, portal first.
func guestReachCandidates(primary string, lists ...[]string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return
		}
		if _, ok := seen[ip]; ok {
			return
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	add(primary)
	for _, list := range lists {
		for _, ip := range list {
			add(ip)
		}
	}
	return out
}

func (h *Handler) bootstrapGuestSSHForChangeIP(ctx context.Context, instanceID, externalID, osTemplateID string, reachCandidates []string, rootPass string) (string, sshavail.GuestSSHAuth, error) {
	auth := guestSSHAuth(rootPass)
	if len(reachCandidates) == 0 {
		return rootPass, auth, nil
	}

	tryAuth := func(a sshavail.GuestSSHAuth) bool {
		_, err := sshavail.DialAnyRootAuth(ctx, reachCandidates, a)
		return err == nil
	}

	if tryAuth(auth) {
		return rootPass, auth, nil
	}
	if auth.Signer != nil && tryAuth(sshavail.GuestSSHAuth{Signer: auth.Signer}) {
		return rootPass, sshavail.GuestSSHAuth{Signer: auth.Signer}, nil
	}

	if h.hv() == nil || externalID == "" {
		return rootPass, auth, nil
	}
	newPass, err := h.hv().ResetRootPassword(ctx, externalID, osTemplateID)
	if err != nil {
		log.Printf("change ip bootstrap reset password %s: %v", instanceID, err)
		return rootPass, auth, nil
	}
	newPass = strings.TrimSpace(newPass)
	if newPass == "" {
		return rootPass, auth, nil
	}
	_ = h.store.UpdateInstanceRootPassword(ctx, instanceID, newPass)
	auth = guestSSHAuth(newPass)
	if tryAuth(auth) {
		if pub := opsssh.AuthorizedKeyLine(); pub != "" {
			if reachIP, err := sshavail.DialAnyRootAuth(ctx, reachCandidates, auth); err == nil {
				_ = sshavail.InstallAuthorizedKeys(ctx, reachIP, newPass, []string{pub}, false)
			}
		}
		return newPass, auth, nil
	}
	if auth.Signer != nil && tryAuth(sshavail.GuestSSHAuth{Signer: auth.Signer}) {
		return newPass, sshavail.GuestSSHAuth{Signer: auth.Signer}, nil
	}
	return newPass, auth, nil
}

func guestSSHAuthEmpty(auth sshavail.GuestSSHAuth) bool {
	return strings.TrimSpace(auth.Password) == "" && auth.Signer == nil
}

func reconfigurePrimaryIPGuest(
	ctx context.Context,
	instanceID string,
	reachCandidates []string,
	newIP, gateway string,
	prefix int,
	auth sshavail.GuestSSHAuth,
) bool {
	if guestSSHAuthEmpty(auth) {
		return false
	}
	if prefix <= 0 || prefix > 32 {
		prefix = 24
	}
	newIP = strings.TrimSpace(newIP)
	candidates := guestReachCandidates(newIP, reachCandidates)

	const attempts = 6
	for i := 1; i <= attempts; i++ {
		if i > 1 {
			if err := sleepCtx(ctx, 4*time.Second); err != nil {
				return false
			}
		}
		reachIP, err := sshavail.DialAnyRootAuth(ctx, candidates, auth)
		if err != nil {
			log.Printf("change ip guest ssh %s attempt %d: %v", instanceID, i, err)
			continue
		}
		reconfigureErr := sshavail.ReconfigureStaticIPv4Auth(ctx, reachIP, auth, newIP, gateway, prefix)
		if reconfigureErr != nil {
			log.Printf("change ip guest reconfigure %s ->%s via %s attempt %d: %v", instanceID, newIP, reachIP, i, reconfigureErr)
		}
		// nohup switches the address ~2s after the SSH command returns
		_ = sleepCtx(ctx, 3*time.Second)
		const verifyAttempts = 10
		for verify := 0; verify < verifyAttempts; verify++ {
			if verify > 0 {
				if err := sleepCtx(ctx, 3*time.Second); err != nil {
					return false
				}
			}
			if err := sshavail.CheckRootAuth(ctx, newIP, auth); err == nil {
				return true
			}
		}
		if reconfigureErr == nil {
			log.Printf("change ip guest verify new address %s (%s) attempt %d: unreachable", instanceID, newIP, i)
		}
	}
	return false
}

// releaseExtraPrimaryIPv4 removes every attached primary IPv4 except keepIP.
func releaseExtraPrimaryIPv4(ctx context.Context, hv hypervisor.Adapter, externalID, keepIP string, attachedIPs []string) {
	keepIP = strings.TrimSpace(keepIP)
	for _, ip := range attachedIPs {
		ip = strings.TrimSpace(ip)
		if ip == "" || ip == keepIP {
			continue
		}
		if err := hv.RemovePrimaryIPv4(ctx, externalID, ip); err != nil {
			log.Printf("change ip release extra %s (%s): %v", externalID, ip, err)
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
