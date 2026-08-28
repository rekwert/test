package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/authn"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/password"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/smtpblock"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) staffOnly(w http.ResponseWriter, r *http.Request) (*authn.Claims, bool) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.jwtSecret)
	if err != nil || !authn.IsStaff(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return nil, false
	}
	return claims, true
}

func (h *Handler) adminOrOwnerOnly(w http.ResponseWriter, r *http.Request) (*authn.Claims, bool) {
	claims, err := authn.ClaimsFromRequest(r.Header.Get("Authorization"), h.jwtSecret)
	if err != nil || !authn.IsAdminOrOwner(claims.Roles) {
		writeError(w, http.StatusForbidden, "forbidden")
		return nil, false
	}
	return claims, true
}

func (h *Handler) AdminListUserInstances(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	userID := r.PathValue("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"instances": []any{}})
		return
	}
	items, err := h.store.ListInstancesAdmin(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list instances")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": instancesToJSON(items)})
}

func (h *Handler) AdminIssueInstance(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.adminOrOwnerOnly(w, r)
	if !ok {
		return
	}
	userID := strings.TrimSpace(r.PathValue("user_id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "orders unavailable")
		return
	}

	var req struct {
		PlanID            string `json:"plan_id"`
		Region            string `json:"region"`
		Days              int    `json:"days"`
		Hostname          string `json:"hostname"`
		OSTemplateID      string `json:"os_template_id"`
		SoftwareProfileID string `json:"software_profile_id"`
		RootPassword      string `json:"root_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.PlanID = strings.TrimSpace(req.PlanID)
	req.Region = strings.TrimSpace(strings.ToLower(req.Region))
	req.Hostname = strings.TrimSpace(strings.ToLower(req.Hostname))
	req.OSTemplateID = strings.TrimSpace(req.OSTemplateID)
	req.SoftwareProfileID = strings.TrimSpace(req.SoftwareProfileID)
	req.RootPassword = strings.TrimSpace(req.RootPassword)

	if req.PlanID == "" || req.Region == "" {
		writeError(w, http.StatusBadRequest, "plan_id and region required")
		return
	}
	if req.Days < 1 || req.Days > store.MaxAdminIssueDays {
		writeError(w, http.StatusBadRequest, "days must be between 1 and 9999")
		return
	}
	if req.Hostname != "" && !hostnameRe.MatchString(req.Hostname) {
		writeError(w, http.StatusBadRequest, "invalid hostname")
		return
	}
	if req.OSTemplateID != "" && !h.osTemplateAvailable(r.Context(), req.OSTemplateID) {
		writeError(w, http.StatusBadRequest, "os_template not available")
		return
	}
	if req.RootPassword != "" && len(req.RootPassword) < 8 {
		writeError(w, http.StatusBadRequest, "root_password must be at least 8 characters")
		return
	}
	if req.RootPassword == "" {
		generated, err := password.GenerateRoot()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "issue failed")
			return
		}
		req.RootPassword = generated
	}

	result, err := h.store.CreateAdminIssuedOrder(r.Context(), store.AdminIssueInput{
		UserID:            userID,
		StaffID:           claims.UserID,
		PlanID:            req.PlanID,
		Region:            req.Region,
		Days:              req.Days,
		Hostname:          req.Hostname,
		RootPassword:      req.RootPassword,
		OSTemplateID:      req.OSTemplateID,
		SoftwareProfileID: req.SoftwareProfileID,
	})
	if err != nil {
		log.Printf("AdminIssueInstance user=%s plan=%s region=%s: %v", userID, req.PlanID, req.Region, err)
		switch {
		case errors.Is(err, store.ErrInvalidIssueDays):
			writeError(w, http.StatusBadRequest, "days must be between 1 and 9999")
		case errors.Is(err, store.ErrBillingSuspended):
			writeError(w, http.StatusForbidden, "billing account suspended")
		case errors.Is(err, store.ErrPlanNotFound):
			writeError(w, http.StatusBadRequest, "plan not available")
		case errors.Is(err, store.ErrNoNodeForRegion):
			writeError(w, http.StatusBadRequest, "region not available")
		case strings.Contains(err.Error(), "plan not available in region"):
			writeError(w, http.StatusBadRequest, "plan not available in region")
		case strings.Contains(err.Error(), "software profile not available"):
			writeError(w, http.StatusBadRequest, "software_profile not available for os")
		default:
			writeError(w, http.StatusInternalServerError, "issue failed")
		}
		return
	}

	msg := "server issued, provisioning started"
	if result.Queued {
		msg = "server issued, waiting for capacity"
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":           true,
		"message":      msg,
		"id":           result.OrderID,
		"order_number": result.OrderNumber,
		"instance_id":  result.InstanceID,
		"user_id":      userID,
		"plan_id":      req.PlanID,
		"region":       req.Region,
		"days":         req.Days,
		"hostname":     req.Hostname,
		"amount":       0,
		"status":       result.Status,
		"queued":       result.Queued,
	})
}

func (h *Handler) AdminSubuserCheck(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	var req struct {
		PterodactylUserID string `json:"pterodactyl_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.PterodactylUserID = strings.TrimSpace(req.PterodactylUserID)
	if req.PterodactylUserID == "" {
		writeError(w, http.StatusBadRequest, "pterodactyl_user_id required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pterodactyl_user_id": req.PterodactylUserID,
		"found":               false,
		"message":             "Pterodactyl integration pending",
	})
}

func (h *Handler) AdminIPCheck(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	var req struct {
		IP      string `json:"ip"`
		SSHPort int    `json:"ssh_port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ip := strings.TrimSpace(req.IP)
	if ip == "" || net.ParseIP(ip) == nil {
		writeError(w, http.StatusBadRequest, "valid ip required")
		return
	}
	writeJSON(w, http.StatusOK, RunIPBlockCheck(r.Context(), ip, IPCheckOptions{SSHPort: req.SSHPort}))
}

func (h *Handler) AdminVMByIP(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))
	if ip == "" || net.ParseIP(ip) == nil {
		writeError(w, http.StatusBadRequest, "valid ip query required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	inst, err := h.store.FindInstanceByIP(r.Context(), ip)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"instance": instanceToJSON(*inst)})
}

func (h *Handler) AdminVMBlock(w http.ResponseWriter, r *http.Request) {
	h.adminVMToggleBlock(w, r, true)
}

func (h *Handler) AdminVMUnblock(w http.ResponseWriter, r *http.Request) {
	h.adminVMToggleBlock(w, r, false)
}

func (h *Handler) adminVMToggleBlock(w http.ResponseWriter, r *http.Request, block bool) {
	claims, ok := h.adminOrOwnerOnly(w, r)
	if !ok {
		return
	}
	var req struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ip := strings.TrimSpace(req.IP)
	if ip == "" || net.ParseIP(ip) == nil {
		writeError(w, http.StatusBadRequest, "valid ip required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	inst, err := h.store.FindInstanceByIP(r.Context(), ip)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	h.adminVMToggleBlockInstance(w, r, inst, block, claims.UserID)
}

func (h *Handler) AdminInstanceBlock(w http.ResponseWriter, r *http.Request) {
	h.adminVMToggleBlockByID(w, r, true)
}

func (h *Handler) AdminInstanceUnblock(w http.ResponseWriter, r *http.Request) {
	h.adminVMToggleBlockByID(w, r, false)
}

func (h *Handler) AdminInstanceSMTPOpen(w http.ResponseWriter, r *http.Request) {
	h.adminInstanceSMTPToggle(w, r, true)
}

func (h *Handler) AdminInstanceSMTPClose(w http.ResponseWriter, r *http.Request) {
	h.adminInstanceSMTPToggle(w, r, false)
}

func (h *Handler) adminInstanceSMTPToggle(w http.ResponseWriter, r *http.Request, open bool) {
	claims, ok := h.adminOrOwnerOnly(w, r)
	if !ok {
		return
	}
	instanceID := strings.TrimSpace(r.PathValue("id"))
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	target, err := h.store.GetSMTPControlTarget(r.Context(), instanceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if target.ProductType == "dedicated" || target.Provider == "hetzner_robot" || target.Provider == "hostkey" {
		writeError(w, http.StatusBadRequest, "smtp port control is only for VPS (VirtFusion)")
		return
	}
	if target.IP == "" {
		writeError(w, http.StatusConflict, "instance has no IP yet")
		return
	}
	if target.HVHost == "" {
		writeError(w, http.StatusConflict, "hypervisor IP unknown for this node")
		return
	}
	if err := smtpblock.SetGuestAllowed(r.Context(), target.HVHost, target.IP, open); err != nil {
		log.Printf("admin smtp outbound %s open=%v hv=%s ip=%s: %v", instanceID, open, target.HVHost, target.IP, err)
		writeError(w, http.StatusBadGateway, "failed to update hypervisor firewall: "+err.Error())
		return
	}
	if err := h.store.SetSmtpOutboundOpen(r.Context(), instanceID, open); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	action := "admin_smtp_close"
	if open {
		action = "admin_smtp_open"
	}
	details, _ := json.Marshal(map[string]any{
		"ip":      target.IP,
		"hv_host": target.HVHost,
		"ports":   []int{25, 2525},
		"open":    open,
	})
	_ = h.store.LogAdminAction(r.Context(), claims.UserID, target.UserID, instanceID, action, details)

	inst := &store.Instance{ID: instanceID, SmtpOutboundOpen: open}
	if full, err := h.store.GetInstanceForUser(r.Context(), target.UserID, instanceID); err == nil {
		inst = full
	} else {
		inst.IPAddress = &target.IP
		inst.SmtpOutboundOpen = open
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"smtp_outbound_open": open,
		"ports":              []int{25, 2525},
		"instance":           instanceToJSON(*inst),
	})
}

func (h *Handler) adminVMToggleBlockByID(w http.ResponseWriter, r *http.Request, block bool) {
	claims, ok := h.adminOrOwnerOnly(w, r)
	if !ok {
		return
	}
	instanceID := strings.TrimSpace(r.PathValue("id"))
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	inst, err := h.store.GetInstanceByID(r.Context(), instanceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	h.adminVMToggleBlockInstance(w, r, inst, block, claims.UserID)
}

func (h *Handler) adminVMToggleBlockInstance(w http.ResponseWriter, r *http.Request, inst *store.Instance, block bool, staffID string) {
	if inst.State == "deleted" {
		writeError(w, http.StatusConflict, "instance deleted")
		return
	}

	userID, _ := h.store.GetInstanceOwner(r.Context(), inst.ID)
	externalID, _ := h.store.GetInstanceExternalID(r.Context(), inst.ID)

	if block {
		if err := h.store.ApplyAdminBlock(r.Context(), inst.ID); err != nil {
			if err == pgx.ErrNoRows {
				writeError(w, http.StatusNotFound, "instance not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		if externalID != "" {
			if err := h.stopInstancePower(r.Context(), inst.ID, externalID); err != nil {
				log.Printf("admin block stop %s vf=%s: %v", inst.ID, externalID, err)
				if userID != "" {
					_ = h.store.EnqueueInstanceStop(r.Context(), inst.ID, externalID, userID, "admin_block")
				}
			}
		}
		inst.State = "stopped"
		inst.AdminBlock = true
		action := "admin_vm_block"
		if userID != "" {
			_ = h.store.LogAdminAction(r.Context(), staffID, userID, inst.ID, action, nil)
		}
	} else {
		if err := h.store.ClearAdminBlock(r.Context(), inst.ID); err != nil {
			if err == pgx.ErrNoRows {
				writeError(w, http.StatusNotFound, "instance not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		inst.AdminBlock = false
		hold, _ := h.store.InstanceAbuseHold(r.Context(), inst.ID)
		if inst.BillingStatus == "active" && !hold && externalID != "" && userID != "" {
			inst.State = "starting"
			if err := h.store.EnqueueInstanceStart(r.Context(), inst.ID, externalID, userID); err != nil {
				log.Printf("admin unblock start outbox %s: %v", inst.ID, err)
			}
		} else {
			inst.State = "stopped"
		}
		action := "admin_vm_unblock"
		if userID != "" {
			_ = h.store.LogAdminAction(r.Context(), staffID, userID, inst.ID, action, nil)
		}
	}

	if userID != "" {
		if full, err := h.store.GetInstanceForUser(r.Context(), userID, inst.ID); err == nil {
			inst = full
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"blocked":  block,
		"instance": instanceToJSON(*inst),
	})
}

func (h *Handler) AdminInstanceChangeIP(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.adminOrOwnerOnly(w, r)
	if !ok {
		return
	}
	instanceID := strings.TrimSpace(r.PathValue("id"))
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	var req struct {
		OldIP    string `json:"old_ip"`
		WaiveFee bool   `json:"waive_fee"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	userID, err := h.store.GetInstanceOwner(r.Context(), instanceID)
	if err != nil || userID == "" {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	result, err := h.executeInstanceChangeIP(r.Context(), userID, instanceID, req.OldIP, claims.UserID, req.WaiveFee)
	if err != nil {
		writeChangeIPExecError(w, err)
		return
	}

	details, _ := json.Marshal(map[string]any{
		"old_ip":         result.OldIP,
		"new_ip":         result.NewIP,
		"waive_fee":      req.WaiveFee,
		"amount_charged": result.AmountCharged,
	})
	_ = h.store.LogAdminAction(r.Context(), claims.UserID, userID, instanceID, "admin_change_ip", details)

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

func (h *Handler) AdminInstanceAction(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.staffOnly(w, r)
	if !ok {
		return
	}
	instanceID := r.PathValue("id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	var action string
	var targetState string
	switch {
	case strings.HasSuffix(r.URL.Path, "/start"):
		action = "start"
		targetState = "running"
	case strings.HasSuffix(r.URL.Path, "/stop"):
		action = "stop"
		targetState = "stopped"
	case strings.HasSuffix(r.URL.Path, "/reboot"):
		action = "reboot"
		targetState = "running"
	case strings.HasSuffix(r.URL.Path, "/delete") || r.Method == http.MethodDelete:
		action = "delete"
		targetState = "deleted"
	default:
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	}

	inst, err := h.store.GetInstanceByID(r.Context(), instanceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if inst.State == "deleted" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"action":   "deleted",
			"instance": instanceToJSON(*inst),
		})
		return
	}
	if action == "delete" {
		externalID, _ := h.store.GetInstanceExternalID(r.Context(), instanceID)
		provider, productType, _, _ := h.store.GetInstanceProvider(r.Context(), instanceID)
		isDedicated := store.IsDedicatedProvider(provider) || productType == "dedicated"

		if err := h.store.MarkInstanceDeletePending(r.Context(), instanceID); err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		// Cancel lifecycle kickoff first. An already-running poll is protected by
		// state-conditional updates and cannot revive a deleted instance.
		if err := h.store.MarkProvisionOutboxPublished(r.Context(), instanceID); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		if err := h.store.MarkReinstallOutboxPublished(r.Context(), instanceID); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		if externalID != "" && !isDedicated {
			if err := h.store.EnqueueInstanceDestroy(r.Context(), instanceID, externalID, claims.UserID); err != nil {
				log.Printf("admin enqueue destroy %s: %v", instanceID, err)
				writeError(w, http.StatusInternalServerError, "delete failed")
				return
			}
		} else if externalID != "" {
			log.Printf("admin dedicated delete local-only %s external=%s", instanceID, externalID)
		}
		if err := h.store.SetInstanceState(r.Context(), instanceID, targetState); err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		_ = h.store.LogAdminAction(r.Context(), claims.UserID, "", instanceID, "vm_delete", nil)
		inst.State = targetState
		status := http.StatusOK
		if externalID != "" && !isDedicated {
			status = http.StatusAccepted
		}
		writeJSON(w, status, map[string]any{
			"ok":       true,
			"action":   targetState,
			"instance": instanceToJSON(*inst),
		})
		return
	}
	if action == "start" || action == "reboot" {
		ok, billErr := h.store.BillingAllowsPowerOn(r.Context(), instanceID)
		if billErr != nil {
			writeError(w, http.StatusInternalServerError, "billing check failed")
			return
		}
		if !ok {
			writeError(w, http.StatusForbidden, "start blocked until client pays")
			return
		}
	}

	externalID, _ := h.store.GetInstanceExternalID(r.Context(), instanceID)
	if externalID != "" {
		hv := h.hv()
		var hvErr error
		switch action {
		case "start":
			hvErr = runPowerWithRetry(r.Context(), func() error {
				return hv.StartServer(r.Context(), externalID)
			})
		case "stop":
			hvErr = runPowerWithRetry(r.Context(), func() error {
				return hv.StopServer(r.Context(), externalID)
			})
		case "reboot":
			hvErr = runPowerWithRetry(r.Context(), func() error {
				return hv.RebootServer(r.Context(), externalID)
			})
		}
		if hvErr != nil {
			log.Printf("admin %s %s: %v", action, instanceID, hvErr)
			if powerBusyError(hvErr) {
				writeError(w, http.StatusConflict, "server is busy, try again shortly")
				return
			}
			writeError(w, http.StatusBadGateway, action+" failed")
			return
		}
	}

	if err := h.store.SetInstanceState(r.Context(), instanceID, targetState); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	actionName := "vm_start"
	switch {
	case action == "stop":
		actionName = "vm_stop"
	case action == "reboot":
		actionName = "vm_reboot"
	case action == "delete":
		actionName = "vm_delete"
	}
	_ = h.store.LogAdminAction(r.Context(), claims.UserID, "", instanceID, actionName, nil)
	inst.State = targetState
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"action":   targetState,
		"instance": instanceToJSON(*inst),
	})
}

func probePort(ip, port string) map[string]any {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, port), 3*time.Second)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return map[string]any{"reachable": false, "error": err.Error()}
	}
	_ = conn.Close()
	return map[string]any{"reachable": true, "latency_ms": latency}
}

func instancesToJSON(items []store.Instance) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, inst := range items {
		out = append(out, instanceToJSON(inst))
	}
	return out
}

func instanceToJSON(inst store.Instance) map[string]any {
	row := map[string]any{
		"id":                  inst.ID,
		"state":               inst.State,
		"region":              inst.Region,
		"plan_id":             inst.PlanID,
		"billing_status":      inst.BillingStatus,
		"billing_period_days": inst.BillingPeriodDays,
	}
	if inst.PlanName != "" {
		row["plan_name"] = inst.PlanName
	}
	if name := strings.TrimSpace(inst.OSName); name != "" {
		row["os_name"] = name
	}
	if sw := strings.TrimSpace(inst.SoftwareProfileID); sw != "" {
		row["software_profile_id"] = sw
	}
	row["price_monthly"] = inst.PriceMonthly
	row["currency"] = "RUB"
	if inst.CPU > 0 {
		row["cpu"] = inst.CPU
	}
	if inst.RAMMb > 0 {
		row["ram_mb"] = inst.RAMMb
	}
	if inst.DiskGB > 0 {
		row["disk_gb"] = inst.DiskGB
	}
	if inst.Hostname != nil && *inst.Hostname != "" {
		row["hostname"] = *inst.Hostname
	}
	if inst.IPAddress != nil && *inst.IPAddress != "" {
		row["ip_address"] = *inst.IPAddress
	}
	if inst.NodeID != nil && *inst.NodeID != "" {
		row["node_id"] = *inst.NodeID
	}
	if inst.NodeName != nil && *inst.NodeName != "" {
		row["node_name"] = *inst.NodeName
	}
	if len(inst.Metrics) > 2 {
		row["metrics"] = json.RawMessage(inst.Metrics)
	}
	if inst.NextBillingAt != nil {
		row["next_billing_at"] = inst.NextBillingAt.UTC().Format(time.RFC3339)
	}
	if inst.CreatedAt != nil {
		row["created_at"] = inst.CreatedAt.UTC().Format(time.RFC3339)
	}
	if inst.OrderNumber != nil && *inst.OrderNumber > 0 {
		row["order_number"] = *inst.OrderNumber
	}
	row["auto_renew"] = inst.AutoRenew
	if inst.ProductType != "" {
		row["product_type"] = inst.ProductType
	}
	if inst.Provider != "" {
		row["provider"] = inst.Provider
	}
	if inst.AdminBlock {
		row["admin_block"] = true
	}
	row["smtp_outbound_open"] = inst.SmtpOutboundOpen
	if len(inst.ProviderMeta) > 2 {
		row["provider_meta"] = json.RawMessage(inst.ProviderMeta)
	}
	return row
}

func (h *Handler) AdminStats(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	period := store.NewStatsPeriod(days, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"users": 0, "instances_active": 0, "instances_running": 0,
			"instances_grace": 0, "instances_creating": 0,
			"instances_error": 0, "instances_not_running": 0,
			"tickets_open": 0, "tickets_stale_24h": 0, "registrations_period": 0,
			"revenue_total": 0, "nodes_online": 0, "nodes_offline": 0, "nodes_total": 0,
			"clients_on_free_plan": 0,
			"period_days": period.Days, "period_from": period.From.Format("2006-01-02"),
			"period_to": period.To.Format("2006-01-02"),
		})
		return
	}
	stats, err := h.store.DashboardStats(r.Context(), period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":                 stats.UsersCount,
		"instances_active":      stats.InstancesActive,
		"instances_running":     stats.InstancesRunning,
		"instances_grace":       stats.InstancesGrace,
		"instances_creating":    stats.InstancesCreating,
		"instances_error":       stats.InstancesError,
		"instances_not_running": stats.InstancesNotRunning,
		"tickets_open":          stats.TicketsOpen,
		"tickets_stale_24h":     stats.TicketsStale24h,
		"registrations_period":  stats.RegistrationsPeriod,
		"revenue_total":         stats.RevenueTotal,
		"nodes_online":          stats.NodesOnline,
		"nodes_offline":         stats.NodesOffline,
		"nodes_total":           stats.NodesTotal,
		"clients_on_free_plan":  stats.ClientsOnFreePlan,
		"period_days":           stats.PeriodDays,
		"period_from":           stats.PeriodFrom,
		"period_to":             stats.PeriodTo,
	})
}

func (h *Handler) AdminListInstances(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"instances": []any{}})
		return
	}
	q := r.URL.Query().Get("q")
	ip := r.URL.Query().Get("ip")
	userID := r.URL.Query().Get("user_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListAllInstances(r.Context(), q, ip, userID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list instances")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, row := range items {
		item := map[string]any{
			"id":             row.ID,
			"user_id":        row.UserID,
			"user_email":     row.UserEmail,
			"state":          row.State,
			"region":         row.Region,
			"plan_id":        row.PlanID,
			"plan_name":      row.PlanName,
			"billing_status": row.BillingStatus,
			"product_type":   row.ProductType,
			"created_at":     row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if row.CPU > 0 {
			item["cpu"] = row.CPU
		}
		if row.RAMMb > 0 {
			item["ram_mb"] = row.RAMMb
		}
		if row.DiskGB > 0 {
			item["disk_gb"] = row.DiskGB
		}
		if row.NextBillingAt != nil {
			item["next_billing_at"] = row.NextBillingAt.UTC().Format(time.RFC3339)
		}
		if row.Hostname != nil && *row.Hostname != "" {
			item["hostname"] = *row.Hostname
		}
		if row.IPAddress != nil && *row.IPAddress != "" {
			item["ip_address"] = *row.IPAddress
		}
		if row.ExternalID != nil && *row.ExternalID != "" {
			item["external_id"] = *row.ExternalID
		}
		if row.NodeID != nil && *row.NodeID != "" {
			item["node_id"] = *row.NodeID
		}
		if row.NodeName != nil && *row.NodeName != "" {
			item["node_name"] = *row.NodeName
		}
		if row.NodeVFName != nil && *row.NodeVFName != "" {
			item["node_vf_name"] = *row.NodeVFName
		}
		if row.OrderID != nil && *row.OrderID != "" {
			item["order_id"] = *row.OrderID
		}
		if row.OrderNumber != nil && *row.OrderNumber > 0 {
			item["order_number"] = *row.OrderNumber
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": out})
}

func (h *Handler) AdminListNodes(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"nodes": []any{}})
		return
	}
	items, err := h.store.ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list nodes")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, n := range items {
		row := map[string]any{
			"id":                 n.ID,
			"name":               n.Name,
			"region":             n.Region,
			"status":             n.Status,
			"capacity_instances": n.CapacityInstances,
			"active_instances":   n.ActiveInstances,
			"pending_instances":  n.PendingInstances,
			"maintenance_mode":   n.MaintenanceMode,
			"supported_tiers":    n.SupportedTiers,
		}
		if n.SupportedTiers == nil {
			row["supported_tiers"] = []string{}
		}
		if n.ExternalID != nil && *n.ExternalID != "" {
			row["external_id"] = *n.ExternalID
		}
		if n.VFName != nil && *n.VFName != "" {
			row["vf_name"] = *n.VFName
		}
		if n.VFIP != nil && *n.VFIP != "" {
			row["vf_ip"] = *n.VFIP
		}
		if n.VFEnabled != nil {
			row["vf_enabled"] = *n.VFEnabled
		}
		if n.MaxCPUCores != nil {
			row["max_cpu_cores"] = *n.MaxCPUCores
		}
		if n.CPUAllocated != nil {
			row["cpu_allocated"] = *n.CPUAllocated
		}
		if n.CPUUsedPercent != nil {
			row["cpu_used_percent"] = *n.CPUUsedPercent
		}
		if n.MaxMemoryMB != nil {
			row["max_memory_mb"] = *n.MaxMemoryMB
		}
		if n.MemoryAllocatedMB != nil {
			row["memory_allocated_mb"] = *n.MemoryAllocatedMB
		}
		if n.MemoryUsedPercent != nil {
			row["memory_used_percent"] = *n.MemoryUsedPercent
		}
		if n.MaxDiskGB != nil {
			row["max_disk_gb"] = *n.MaxDiskGB
		}
		if n.DiskAllocatedGB != nil {
			row["disk_allocated_gb"] = *n.DiskAllocatedGB
		}
		if n.DiskUsedPercent != nil {
			row["disk_used_percent"] = *n.DiskUsedPercent
		}
		if n.VFServerCount != nil {
			row["vf_server_count"] = *n.VFServerCount
		}
		if n.LastSyncedAt != nil {
			row["last_synced_at"] = n.LastSyncedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": out})
}

func (h *Handler) AdminUpdateNodeTiers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "node id required")
		return
	}
	var req struct {
		SupportedTiers []string `json:"supported_tiers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.store.UpdateNodeSupportedTiers(r.Context(), nodeID, req.SupportedTiers); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		if strings.Contains(err.Error(), "no valid supported tiers") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("AdminUpdateNodeTiers: %v", err)
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": nodeID, "supported_tiers": req.SupportedTiers})
}

func (h *Handler) AdminListRegionTiers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"region_tiers": []any{}})
		return
	}
	items, err := h.store.ListRegionTiers(r.Context())
	if err != nil {
		log.Printf("AdminListRegionTiers: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list region tiers")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"region":  item.Region,
			"tier":    item.Tier,
			"enabled": item.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region_tiers": out})
}

func (h *Handler) AdminUpdateRegionTier(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	region := strings.TrimSpace(strings.ToLower(r.PathValue("region")))
	tier := strings.TrimSpace(strings.ToLower(r.PathValue("tier")))
	if region == "" || tier == "" {
		writeError(w, http.StatusBadRequest, "region and tier required")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.store.SetRegionTierEnabled(r.Context(), region, tier, req.Enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "region tier not found")
			return
		}
		if strings.Contains(err.Error(), "unknown tier") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("AdminUpdateRegionTier: %v", err)
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"region":  region,
		"tier":    tier,
		"enabled": req.Enabled,
	})
}

func (h *Handler) AdminSuspendClient(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.adminOrOwnerOnly(w, r)
	if !ok {
		return
	}
	userID := r.PathValue("user_id")
	if userID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	if err := h.store.SuspendClient(r.Context(), userID, claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "suspend failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "billing_status": "suspended"})
}

func (h *Handler) AdminUnsuspendClient(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.staffOnly(w, r)
	if !ok {
		return
	}
	userID := r.PathValue("user_id")
	if userID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	if err := h.store.UnsuspendClient(r.Context(), userID, claims.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "unsuspend failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "billing_status": "active"})
}

func (h *Handler) AdminUpgradeQuote(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	instanceID := r.PathValue("id")
	if instanceID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	planID := strings.TrimSpace(r.URL.Query().Get("plan_id"))
	if planID == "" {
		writeError(w, http.StatusBadRequest, "plan_id required")
		return
	}
	userID, err := h.store.GetInstanceOwner(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	quote, err := h.store.GetUpgradeQuoteForUser(r.Context(), userID, instanceID, planID)
	if err != nil {
		writeUpgradeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, upgradeQuoteToJSON(quote))
}

func (h *Handler) AdminUpgradeInstance(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.adminOrOwnerOnly(w, r)
	if !ok {
		return
	}
	instanceID := r.PathValue("id")
	if instanceID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "instance id required")
		return
	}
	var req struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.PlanID) == "" {
		writeError(w, http.StatusBadRequest, "plan_id required")
		return
	}
	userID, err := h.store.GetInstanceOwner(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	result, err := h.executeInstanceUpgrade(r.Context(), userID, instanceID, strings.TrimSpace(req.PlanID), claims.UserID)
	if err != nil {
		writeUpgradeExecError(w, err)
		return
	}
	row := instanceToJSON(*result.Instance)
	row["amount_charged"] = result.Amount
	row["from_plan"] = result.FromPlan
	row["to_plan"] = result.ToPlan
	row["rebooted"] = true
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "instance": row, "amount_charged": result.Amount})
}

func (h *Handler) AdminClientActions(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	userID := r.PathValue("user_id")
	if userID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListAdminActions(r.Context(), userID, "", limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load actions")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, a := range items {
		row := map[string]any{
			"id":         a.ID,
			"action":     a.Action,
			"details":    json.RawMessage(a.Details),
			"created_at": a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if a.StaffID != nil {
			row["staff_id"] = *a.StaffID
		}
		if a.InstanceID != nil {
			row["instance_id"] = *a.InstanceID
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": out})
}

func (h *Handler) AdminClientIPHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.staffOnly(w, r); !ok {
		return
	}
	userID := r.PathValue("user_id")
	if userID == "" || h.store == nil {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListClientIPHistory(r.Context(), userID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load ip history")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, row := range items {
		item := map[string]any{
			"id":         row.ID,
			"ip_address": row.IPAddress,
			"event":      row.Event,
			"source":     row.Source,
			"created_at": row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if row.InstanceID != nil {
			item["instance_id"] = *row.InstanceID
		}
		if row.OldIP != nil {
			item["old_ip"] = *row.OldIP
		}
		if row.ActorID != nil {
			item["actor_id"] = *row.ActorID
		}
		if len(row.Metadata) > 0 {
			item["metadata"] = json.RawMessage(row.Metadata)
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
