package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hetznerrobot"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hostkey"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/vpsipv4"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) hv() hypervisor.Adapter {
	if h.hypervisor != nil {
		return h.hypervisor
	}
	return hypervisor.NewMock()
}

func (h *Handler) InstanceExtend(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	var req struct {
		Months int `json:"months"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Months < 1 {
		writeError(w, http.StatusBadRequest, "months required")
		return
	}
	inst, amount, err := h.store.ExtendInstanceMonths(r.Context(), userID, instanceID, req.Months)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInstanceNotFound):
			writeError(w, http.StatusNotFound, "instance not found")
		case errors.Is(err, store.ErrInsufficientBalance):
			writeError(w, http.StatusPaymentRequired, "insufficient balance")
		case errors.Is(err, store.ErrBillingSuspended):
			writeError(w, http.StatusForbidden, "billing account suspended")
		case errors.Is(err, store.ErrInvalidPeriod):
			writeError(w, http.StatusBadRequest, "invalid months")
		case strings.Contains(err.Error(), "renewal not allowed"):
			writeError(w, http.StatusForbidden, "renewal not allowed")
		default:
			writeError(w, http.StatusInternalServerError, "extend failed")
		}
		return
	}
	row := instanceToJSON(*inst)
	row["amount_charged"] = amount
	writeJSON(w, http.StatusOK, row)
}

func (h *Handler) InstanceUpgradeQuote(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	planID := strings.TrimSpace(r.URL.Query().Get("plan_id"))
	if planID == "" {
		writeError(w, http.StatusBadRequest, "plan_id required")
		return
	}
	quote, err := h.store.GetUpgradeQuoteForUser(r.Context(), userID, instanceID, planID)
	if err != nil {
		writeUpgradeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, upgradeQuoteToJSON(quote))
}

func (h *Handler) InstanceUpgrade(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	var req struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.PlanID) == "" {
		writeError(w, http.StatusBadRequest, "plan_id required")
		return
	}
	result, err := h.executeInstanceUpgrade(r.Context(), userID, instanceID, strings.TrimSpace(req.PlanID), "")
	if err != nil {
		writeUpgradeExecError(w, err)
		return
	}
	row := instanceToJSON(*result.Instance)
	row["amount_charged"] = result.Amount
	row["from_plan"] = result.FromPlan
	row["to_plan"] = result.ToPlan
	row["rebooted"] = true
	writeJSON(w, http.StatusOK, row)
}

func (h *Handler) executeInstanceUpgrade(ctx context.Context, userID, instanceID, targetPlanID, staffID string) (*store.UpgradeResult, error) {
	inst, err := h.store.GetInstanceForUser(ctx, userID, instanceID)
	if err != nil {
		return nil, upgradeExecError{status: http.StatusNotFound, msg: "instance not found"}
	}
	if inst.State == "creating" || inst.State == "queued" || inst.State == "reinstalling" || inst.State == "deleted" {
		return nil, upgradeExecError{status: http.StatusConflict, msg: "server is not ready"}
	}
	provider := strings.TrimSpace(strings.ToLower(inst.Provider))
	if provider != "" && provider != "openstack" {
		return nil, upgradeExecError{status: http.StatusBadRequest, msg: "plan change not supported for this server type"}
	}
	externalID, err := h.store.GetInstanceExternalID(ctx, instanceID)
	if err != nil || externalID == "" {
		return nil, upgradeExecError{status: http.StatusConflict, msg: "server is still provisioning"}
	}

	if strings.EqualFold(inst.PlanID, targetPlanID) {
		if !hypervisor.MockEnabled() {
			if hypervisor.ActivePlanMapLen() > 0 && !hypervisor.HasExplicitPlan(targetPlanID) {
				return nil, upgradeExecError{status: http.StatusBadRequest, msg: "plan not available"}
			}
		}
		if err := syncPlanWithReboot(h, ctx, externalID, targetPlanID); err != nil {
			log.Printf("upgrade resize retry %s -> %s: %v", instanceID, targetPlanID, err)
			return nil, upgradeExecError{status: http.StatusBadGateway, msg: upgradeResizeErrorMessage(err)}
		}
		fresh, err := h.store.GetInstanceForUser(ctx, userID, instanceID)
		if err != nil {
			return &store.UpgradeResult{Instance: inst}, nil
		}
		return &store.UpgradeResult{Instance: fresh}, nil
	}

	if _, err := h.store.ValidateUpgradeForUser(ctx, userID, instanceID, targetPlanID); err != nil {
		return nil, err
	}
	if !hypervisor.MockEnabled() {
		if hypervisor.ActivePlanMapLen() > 0 && !hypervisor.HasExplicitPlan(targetPlanID) {
			return nil, upgradeExecError{status: http.StatusBadRequest, msg: "plan not available"}
		}
	}

	result, err := h.store.UpgradeInstanceForUser(ctx, userID, instanceID, targetPlanID, staffID)
	if err != nil {
		return nil, err
	}

	if err := syncPlanWithReboot(h, ctx, externalID, targetPlanID); err != nil {
		log.Printf("upgrade resize after charge %s -> %s: %v", instanceID, targetPlanID, err)
		if rbErr := h.store.RevertUpgradeInstance(ctx, userID, instanceID, result.FromPlanID, result.Amount, "hypervisor resize failed after charge"); rbErr != nil {
			log.Printf("upgrade billing rollback %s: %v", instanceID, rbErr)
		}
		if rbErr := syncPlanWithReboot(h, ctx, externalID, result.FromPlanID); rbErr != nil {
			log.Printf("upgrade hv rollback %s -> %s: %v", instanceID, result.FromPlanID, rbErr)
		}
		return nil, upgradeExecError{status: http.StatusBadGateway, msg: upgradeResizeErrorMessage(err)}
	}
	return result, nil
}

type upgradeExecError struct {
	status int
	msg    string
}

func (e upgradeExecError) Error() string { return e.msg }

func writeUpgradeExecError(w http.ResponseWriter, err error) {
	var execErr upgradeExecError
	if errors.As(err, &execErr) {
		writeError(w, execErr.status, execErr.msg)
		return
	}
	writeUpgradeStoreError(w, err)
}

func upgradeQuoteToJSON(q *store.UpgradeQuote) map[string]any {
	row := map[string]any{
		"amount":              q.Amount,
		"delta_monthly":       q.DeltaMonthly,
		"remaining_days":      q.RemainingDays,
		"billing_period_days": q.BillingPeriodDays,
		"from_plan":           q.FromPlan,
		"to_plan":             q.ToPlan,
		"from_plan_id":        q.FromPlanID,
		"to_plan_id":          q.ToPlanID,
	}
	if q.NextBillingAt != nil {
		row["next_billing_at"] = q.NextBillingAt.UTC().Format(time.RFC3339)
	}
	return row
}

func syncPlanWithReboot(h *Handler, ctx context.Context, externalID, planID string) error {
	if err := h.hv().SyncServerPlan(ctx, externalID, planID); err != nil {
		return err
	}
	if err := h.hv().RebootServer(ctx, externalID); err != nil {
		return fmt.Errorf("package updated but reboot failed: %w", err)
	}
	return nil
}

func writeUpgradeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInstanceNotFound):
		writeError(w, http.StatusNotFound, "instance or plan not found")
	case errors.Is(err, store.ErrSamePlan):
		writeError(w, http.StatusBadRequest, "same plan")
	case errors.Is(err, store.ErrDifferentPlanLine):
		writeError(w, http.StatusBadRequest, "upgrade only within the same plan line")
	case errors.Is(err, store.ErrDowngradeNotAllowed):
		writeError(w, http.StatusBadRequest, "only upgrades are allowed")
	case errors.Is(err, store.ErrDiskShrinkNotAllowed):
		writeError(w, http.StatusBadRequest, "disk shrink not allowed")
	case errors.Is(err, store.ErrPlanRegionMismatch):
		writeError(w, http.StatusBadRequest, "plan not available in this region")
	case errors.Is(err, store.ErrInsufficientBalance):
		writeError(w, http.StatusPaymentRequired, "insufficient balance")
	case errors.Is(err, store.ErrBillingSuspended):
		writeError(w, http.StatusForbidden, "billing account suspended")
	default:
		writeError(w, http.StatusInternalServerError, "upgrade failed")
	}
}

func upgradeResizeErrorMessage(err error) string {
	if err == nil {
		return "hypervisor resize failed"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not enough") ||
		strings.Contains(msg, "insufficient") ||
		strings.Contains(msg, "no capacity") ||
		strings.Contains(msg, "capacity") ||
		strings.Contains(msg, "resource") ||
		strings.Contains(msg, "no space") ||
		strings.Contains(msg, "out of memory") ||
		strings.Contains(msg, "memory") && strings.Contains(msg, "available") ||
		strings.Contains(msg, "storage") && (strings.Contains(msg, "full") || strings.Contains(msg, "space")) ||
		strings.Contains(msg, "unable to allocate") ||
		strings.Contains(msg, "overcommit") {
		return "hypervisor capacity unavailable"
	}
	return "hypervisor resize failed"
}

func (h *Handler) InstanceChangeIP(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	var req struct {
		OldIP string `json:"old_ip"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	result, err := h.executeInstanceChangeIP(r.Context(), userID, instanceID, req.OldIP, userID, false)
	if err != nil {
		writeChangeIPExecError(w, err)
		return
	}
	if result.Instance == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             true,
			"old_ip":         result.OldIP,
			"ip_address":     result.NewIP,
			"amount_charged": result.AmountCharged,
			"guest_updated":  result.GuestUpdated,
		})
		return
	}
	row := instanceToJSON(*result.Instance)
	row["ok"] = true
	row["old_ip"] = result.OldIP
	row["amount_charged"] = result.AmountCharged
	row["guest_updated"] = result.GuestUpdated
	writeJSON(w, http.StatusOK, row)
}

func (h *Handler) InstanceRentIPv4(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	var req struct {
		Quantity int `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Quantity <= 0 {
		writeError(w, http.StatusBadRequest, "invalid quantity")
		return
	}

	ipCfg := vpsipv4.Load()
	if req.Quantity > ipCfg.MaxQty {
		writeError(w, http.StatusBadRequest, "invalid quantity")
		return
	}

	provider, productType, externalID, err := h.store.GetInstanceProvider(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if productType == "dedicated" || store.IsDedicatedProvider(provider) {
		writeError(w, http.StatusConflict, "not available for this server type")
		return
	}
	if externalID == "" {
		writeError(w, http.StatusConflict, "server is still provisioning")
		return
	}

	inst, err := h.store.GetInstanceForUser(r.Context(), userID, instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	existingExtra := vpsipv4.ExtraCountOnInstance(inst.ProviderMeta)
	if existingExtra+req.Quantity > ipCfg.MaxQty {
		writeError(w, http.StatusBadRequest, "extra ipv4 limit reached")
		return
	}
	if inst.State == "creating" || inst.State == "queued" || inst.State == "reinstalling" || inst.State == "deleted" {
		writeError(w, http.StatusConflict, "server is not ready")
		return
	}
	if inst.IPAddress == nil || strings.TrimSpace(*inst.IPAddress) == "" {
		writeError(w, http.StatusConflict, "server has no ip yet")
		return
	}

	amount := ipCfg.OrderChargeRub(req.Quantity)
	if err := h.store.ChargeExtraIPv4Fee(r.Context(), userID, instanceID, amount, req.Quantity); err != nil {
		switch {
		case errors.Is(err, store.ErrInsufficientBalance):
			writeError(w, http.StatusPaymentRequired, "insufficient balance")
		case errors.Is(err, store.ErrBillingSuspended):
			writeError(w, http.StatusForbidden, "billing account suspended")
		default:
			writeError(w, http.StatusInternalServerError, "charge failed")
		}
		return
	}

	hv := h.hv()
	newIPs, err := hv.AddExtraIPv4(r.Context(), externalID, req.Quantity)
	if err != nil {
		log.Printf("rent ipv4 assign %s qty=%d: %v", instanceID, req.Quantity, err)
		_ = h.store.RefundExtraIPv4Fee(r.Context(), userID, instanceID, amount, req.Quantity, "")
		msg := "ip assign failed"
		if isNoFreeIPError(err) {
			msg = "no free ip addresses available"
		}
		writeError(w, http.StatusBadGateway, msg)
		return
	}

	allIPs, _, _, err := hv.PrimaryIPv4Info(r.Context(), externalID)
	if err != nil || len(allIPs) == 0 {
		allIPs = append([]string{strings.TrimSpace(*inst.IPAddress)}, newIPs...)
	}
	if err := h.store.SetInstanceAllIPs(r.Context(), instanceID, allIPs); err != nil {
		log.Printf("rent ipv4 save all_ips %s: %v", instanceID, err)
	}
	ipLogOpts := &store.IPAssignmentLogOpts{
		Source:  store.IPSourceExtraIPv4,
		ActorID: userID,
		Metadata: map[string]any{
			"quantity": req.Quantity,
		},
	}
	for _, ip := range newIPs {
		_ = h.store.LogIPAssigned(r.Context(), userID, instanceID, ip, "", ipLogOpts)
	}

	guestSynced := false
	if creds, credErr := h.store.GetInstanceCredentials(r.Context(), userID, instanceID); credErr == nil {
		rootPass := strings.TrimSpace(creds.RootPassword)
		primaryIP := strings.TrimSpace(*inst.IPAddress)
		if rootPass != "" {
			_, gateway, _, gwErr := hv.PrimaryIPv4Info(r.Context(), externalID)
			if gwErr == nil {
				reachIP, dialErr := sshavail.DialAnyRoot(r.Context(), append([]string{primaryIP}, allIPs...), rootPass)
				if dialErr == nil {
					if syncErr := sshavail.ApplyAllIPv4OnGuest(r.Context(), reachIP, rootPass, primaryIP, gateway, 24, allIPs); syncErr != nil {
						log.Printf("rent ipv4 guest network sync %s via %s: %v", instanceID, reachIP, syncErr)
					} else {
						guestSynced = true
					}
				} else {
					log.Printf("rent ipv4 guest ssh unreachable %s: %v", instanceID, dialErr)
				}
			}
		}
	}

	inst, err = h.store.GetInstanceForUser(r.Context(), userID, instanceID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             true,
			"new_ips":        newIPs,
			"all_ips":        allIPs,
			"amount_charged": amount,
			"guest_synced":   guestSynced,
		})
		return
	}
	row := instanceToJSON(*inst)
	row["ok"] = true
	row["new_ips"] = newIPs
	row["all_ips"] = allIPs
	row["amount_charged"] = amount
	row["guest_synced"] = guestSynced
	writeJSON(w, http.StatusOK, row)
}

func (h *Handler) InstanceSetAutoRenew(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	var req struct {
		AutoRenew *bool `json:"auto_renew"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AutoRenew == nil {
		writeError(w, http.StatusBadRequest, "auto_renew required")
		return
	}
	inst, err := h.store.SetInstanceAutoRenew(r.Context(), userID, instanceID, *req.AutoRenew)
	if err != nil {
		if err == pgx.ErrNoRows || strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		if strings.Contains(err.Error(), "renewal not allowed") {
			writeError(w, http.StatusForbidden, "renewal not allowed")
			return
		}
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, instanceToJSON(*inst))
}

func (h *Handler) InstanceRename(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	var req struct {
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	hostname := strings.ToLower(strings.TrimSpace(req.Hostname))
	if hostname == "" || !hostnameRe.MatchString(hostname) {
		writeError(w, http.StatusBadRequest, "invalid hostname")
		return
	}
	inst, err := h.store.SetInstanceHostname(r.Context(), userID, instanceID, hostname)
	if err != nil {
		if err == pgx.ErrNoRows || strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "rename failed")
		return
	}
	writeJSON(w, http.StatusOK, instanceToJSON(*inst))
}

func (h *Handler) InstanceConsole(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	if !h.requireBillingAccess(w, r, instanceID) {
		return
	}
	externalID, err := h.store.GetInstanceExternalID(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	session, err := h.hv().GetConsole(r.Context(), externalID)
	if err != nil {
		log.Printf("console %s: %v", instanceID, err)
		writeError(w, http.StatusBadGateway, "console unavailable")
		return
	}
	_ = userID
	// Same-origin WebSocket proxy — browser never connects to VirtFusion directly.
	session.URL = h.consoleProxyWSURL(r, instanceID)
	session.Token = ""
	writeJSON(w, http.StatusOK, session)
}

func (h *Handler) InstanceMetrics(w http.ResponseWriter, r *http.Request) {
	_, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	raw, updatedAt, err := h.store.GetInstanceMetricsRaw(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if len(raw) > 2 {
		out := map[string]any{"metrics": json.RawMessage(raw)}
		if updatedAt != nil {
			out["updated_at"] = updatedAt.UTC()
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	externalID, _ := h.store.GetInstanceExternalID(r.Context(), instanceID)
	metrics, err := h.hv().GetMetrics(r.Context(), externalID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "metrics unavailable")
		return
	}
	b, _ := json.Marshal(metrics)
	_ = h.store.UpdateInstanceMetrics(r.Context(), instanceID, b)
	writeJSON(w, http.StatusOK, map[string]any{
		"metrics":    metrics,
		"updated_at": metrics.UpdatedAt,
	})
}

func (h *Handler) InstanceReinstall(w http.ResponseWriter, r *http.Request) {
	userID, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	var req struct {
		OSTemplateID string   `json:"os_template_id"`
		RootPassword string   `json:"root_password"`
		SSHKeyIDs    []string `json:"ssh_key_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.OSTemplateID = strings.TrimSpace(req.OSTemplateID)
	if req.OSTemplateID == "" {
		writeError(w, http.StatusBadRequest, "os_template_id required")
		return
	}

	provider, _, externalID, err := h.store.GetInstanceProvider(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if provider == "hetzner_robot" {
		if externalID == "" {
			writeError(w, http.StatusConflict, "server not ready")
			return
		}
		robot := h.robot
		if robot == nil {
			robot = hetznerrobot.NewFromEnv()
		}
		pass, actErr := robot.ActivateLinux(r.Context(), externalID, req.OSTemplateID, "en")
		if actErr != nil {
			writeError(w, http.StatusBadGateway, "reinstall failed")
			return
		}
		_ = robot.Reset(r.Context(), externalID, "hw")
		if pass != "" {
			_ = h.store.UpdateInstanceRootPassword(r.Context(), instanceID, pass)
		}
		_ = h.store.SetInstanceProviderMeta(r.Context(), instanceID, map[string]any{
			"os_template_id": req.OSTemplateID,
			"os_activated":   true,
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "reinstall_started", "root_password": pass})
		return
	}
	if provider == "hostkey" {
		if externalID == "" {
			writeError(w, http.StatusConflict, "server not ready")
			return
		}
		hk := h.hostkey
		if hk == nil {
			hk = hostkey.NewFromEnv()
		}
		loc := "NL"
		if inst, instErr := h.store.GetInstanceForUser(r.Context(), userID, instanceID); instErr == nil && inst.Region != "" {
			loc = strings.ToUpper(strings.TrimSpace(inst.Region))
			if loc == "GB" {
				loc = "UK"
			}
		}
		osID, _ := strconv.Atoi(strings.TrimSpace(req.OSTemplateID))
		if osID <= 0 {
			images, _ := hk.ListOS(r.Context(), 0)
			osID = hostkey.ResolveOSID(req.OSTemplateID, images)
		}
		pass := strings.TrimSpace(req.RootPassword)
		if pass == "" {
			writeError(w, http.StatusBadRequest, "root_password required")
			return
		}
		_, actErr := hk.Reinstall(r.Context(), hostkey.ReinstallRequest{
			ServerID:     externalID,
			Location:     loc,
			OSID:         osID,
			RootPassword: pass,
		})
		if actErr != nil {
			writeError(w, http.StatusBadGateway, "reinstall failed")
			return
		}
		_ = h.store.UpdateInstanceRootPassword(r.Context(), instanceID, pass)
		_ = h.store.SetInstanceProviderMeta(r.Context(), instanceID, map[string]any{
			"os_template_id": req.OSTemplateID,
			"hostkey_os_id":  osID,
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "reinstall_started", "root_password": pass})
		return
	}

	if !h.osTemplateAvailable(r.Context(), req.OSTemplateID) {
		writeError(w, http.StatusBadRequest, "os_template not available")
		return
	}
	if _, ok := catalog.SoftwareForOSOrClean(req.OSTemplateID); !ok {
		writeError(w, http.StatusBadRequest, "unknown os_template_id")
		return
	}
	sshKeys, err := h.store.ListSSHPublicKeys(r.Context(), userID, req.SSHKeyIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ssh keys failed")
		return
	}
	if strings.TrimSpace(req.RootPassword) == "" {
		if creds, err := h.store.GetInstanceCredentials(r.Context(), userID, instanceID); err == nil {
			req.RootPassword = creds.RootPassword
		}
	}
	inst, err := h.store.GetInstanceForUser(r.Context(), userID, instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	switch strings.ToLower(inst.State) {
	case "creating", "queued", "reinstalling", "deleted":
		writeError(w, http.StatusConflict, "server is busy")
		return
	}
	planName := inst.PlanName
	planTier := ""
	if plan, ok := catalog.PlanByID(inst.PlanID); ok {
		if planName == "" {
			planName = plan.Name
		}
		planTier = plan.Tier
	}
	if !catalog.OSAllowedForPlan(planName, planTier, req.OSTemplateID) {
		writeError(w, http.StatusBadRequest, "os_template not available for this plan")
		return
	}
	if err := h.store.BeginReinstall(r.Context(), instanceID, req.OSTemplateID, req.RootPassword, sshKeys); err != nil {
		if strings.Contains(err.Error(), "not available") {
			writeError(w, http.StatusConflict, "server is busy")
			return
		}
		writeError(w, http.StatusInternalServerError, "queue failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "reinstall_queued"})
}

func (h *Handler) ListInstanceSnapshots(w http.ResponseWriter, r *http.Request) {
	_, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	items, err := h.store.ListSnapshots(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, row := range items {
		item := map[string]any{
			"id":         row.ID,
			"name":       row.Name,
			"status":     row.Status,
			"created_at": row.CreatedAt.UTC(),
		}
		if row.SizeGB != nil {
			item["size_gb"] = *row.SizeGB
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": out})
}

func (h *Handler) CreateInstanceSnapshot(w http.ResponseWriter, r *http.Request) {
	_, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = "snapshot"
	}
	externalID, err := h.store.GetInstanceExternalID(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	snap, err := h.hv().CreateSnapshot(r.Context(), externalID, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "snapshot failed")
		return
	}
	row, err := h.store.InsertSnapshot(r.Context(), instanceID, req.Name, snap.Status, snap.ID, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     row.ID,
		"name":   row.Name,
		"status": row.Status,
	})
}

func (h *Handler) DeleteInstanceSnapshot(w http.ResponseWriter, r *http.Request) {
	_, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	snapID := r.PathValue("snapshot_id")
	vfSnapID, err := h.store.GetSnapshotExternalID(r.Context(), instanceID, snapID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	externalID, _ := h.store.GetInstanceExternalID(r.Context(), instanceID)
	if externalID != "" && vfSnapID != "" {
		if err := h.hv().DeleteSnapshot(r.Context(), externalID, vfSnapID); err != nil {
			log.Printf("delete snapshot vf %s snap=%s: %v", instanceID, vfSnapID, err)
			writeError(w, http.StatusBadGateway, "snapshot delete failed")
			return
		}
	}
	if err := h.store.DeleteSnapshot(r.Context(), instanceID, snapID); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) ListInstanceBackups(w http.ResponseWriter, r *http.Request) {
	_, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	items, err := h.store.ListBackups(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, row := range items {
		item := map[string]any{
			"id":         row.ID,
			"name":       row.Name,
			"status":     row.Status,
			"created_at": row.CreatedAt.UTC(),
		}
		if row.Schedule != nil {
			item["schedule"] = *row.Schedule
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": out})
}

func (h *Handler) CreateInstanceBackup(w http.ResponseWriter, r *http.Request) {
	_, instanceID, ok := h.instanceAccess(w, r)
	if !ok {
		return
	}
	var req struct {
		Name     string `json:"name"`
		Schedule string `json:"schedule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = "backup"
	}
	externalID, _ := h.store.GetInstanceExternalID(r.Context(), instanceID)
	row, err := h.store.InsertBackup(r.Context(), instanceID, req.Name, "ready", externalID+"-backup", req.Schedule)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backup failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     row.ID,
		"name":   row.Name,
		"status": row.Status,
	})
}

func (h *Handler) instanceAccess(w http.ResponseWriter, r *http.Request) (userID, instanceID string, ok bool) {
	userID, ok = h.requireCustomer(w, r, "vps.manage")
	if !ok {
		return "", "", false
	}
	instanceID = r.PathValue("id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance id required")
		return "", "", false
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return "", "", false
	}
	if _, err := h.store.GetInstanceForUser(r.Context(), userID, instanceID); err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return "", "", false
	}
	return userID, instanceID, true
}

func ipOnServer(ips []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, ip := range ips {
		if strings.TrimSpace(ip) == target {
			return true
		}
	}
	return false
}

func isNoFreeIPError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no free") ||
		strings.Contains(msg, "not enough addresses") ||
		strings.Contains(msg, "not enough address") ||
		strings.Contains(msg, "no available") ||
		strings.Contains(msg, "addresses available") ||
		strings.Contains(msg, "insufficient") && strings.Contains(msg, "address")
}

